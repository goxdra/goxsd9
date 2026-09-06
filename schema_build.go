package goxsd9

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	invalidSchemaTargetNamespaceCode               = "XSD3009"
	invalidSchemaCompositionCode                   = "XSD3010"
	invalidSchemaDeclarationNameCode               = "XSD3011"
	diagnosticSchemaSimpleTypeUnresolvedCode       = "XSD3014"
	diagnosticSchemaSimpleTypeWrongKindCode        = "XSD3015"
	diagnosticSchemaSimpleTypeAmbiguousCode        = "XSD3016"
	diagnosticSchemaSimpleTypeCycleCode            = "XSD3017"
	diagnosticSchemaSimpleTypeBaseCode             = "XSD3018"
	diagnosticSchemaElementTypeUnresolvedCode      = "XSD3019"
	diagnosticSchemaElementTypeWrongKindCode       = "XSD3020"
	diagnosticSchemaElementTypeAmbiguousCode       = "XSD3021"
	diagnosticSchemaElementTypeUnsupportedCode     = "XSD3022"
	diagnosticSchemaGlobalDuplicateCode            = "XSD3023"
	diagnosticSchemaElementDuplicateCode           = diagnosticSchemaGlobalDuplicateCode
	diagnosticSchemaElementReferenceUnresolvedCode = "XSD3024"
	diagnosticSchemaElementReferenceWrongKindCode  = "XSD3025"
	diagnosticSchemaElementReferenceAmbiguousCode  = "XSD3026"
	diagnosticSchemaElementReferenceNamespaceCode  = "XSD3027"
	diagnosticSchemaElementReferenceDuplicateCode  = "XSD3028"
	diagnosticSchemaPrecisionDecimalVersionCode    = "XSD3030"
	diagnosticSchemaAllOccurrenceVersionCode       = diagnosticSchemaPrecisionDecimalVersionCode
	diagnosticSchemaNotationCode                   = "XSD3031"
	diagnosticSchemaAttributeTypeUnresolvedCode    = "XSD3032"
	diagnosticSchemaAttributeTypeWrongKindCode     = "XSD3033"
	diagnosticSchemaAttributeTypeAmbiguousCode     = "XSD3034"
	diagnosticSchemaAttributeTypeCycleCode         = "XSD3035"
	diagnosticSchemaSubstitutionUnresolvedCode     = "XSD3037"
	diagnosticSchemaSubstitutionWrongKindCode      = "XSD3038"
	diagnosticSchemaSubstitutionAmbiguousCode      = "XSD3039"
	diagnosticSchemaSubstitutionSelfCode           = "XSD3040"
	diagnosticSchemaSubstitutionImportCode         = "XSD3042"
	diagnosticSchemaSubstitutionTypeCode           = "XSD3043"
	diagnosticSchemaSubstitutionCycleCode          = "XSD3044"
	diagnosticSchemaBlockCode                      = "XSD3045"
	diagnosticSchemaElementReferenceBlockCode      = "XSD3046"
	diagnosticSchemaBridgeInvariantCode            = "GOXSD9025"
)

const (
	schemaSimpleTypeXSD10SpecRef                = "xsd10-structures#Simple_Type_Definitions"
	schemaSimpleTypeXSD11SpecRef                = "xsd11-structures#Simple_Type_Definition"
	schemaElementTypeXSD10SpecRef               = "xsd10-structures#Element_Declaration_details"
	schemaElementTypeXSD11SpecRef               = "xsd11-structures#Element_Declaration_details"
	schemaElementTargetNamespaceXSD11SpecRef    = "xsd11-structures#dcl.elt.local"
	schemaBooleanDatatypeXSD10SpecRef           = "xsd10-datatypes#boolean"
	schemaBooleanDatatypeXSD11SpecRef           = "xsd11-datatypes#boolean"
	schemaGlobalDuplicateXSD10SpecRef           = "xsd10-structures#c-nmd"
	schemaGlobalDuplicateXSD11SpecRef           = "xsd11-structures#c-nmd"
	schemaElementDuplicateXSD10SpecRef          = schemaGlobalDuplicateXSD10SpecRef
	schemaElementDuplicateXSD11SpecRef          = schemaGlobalDuplicateXSD11SpecRef
	schemaElementReferenceXSD10SpecRef          = "xsd10-structures#src-resolve"
	schemaElementReferenceXSD11SpecRef          = "xsd11-structures#src-resolve"
	schemaElementReferenceBlockXSD10SpecRef     = "xsd10-structures#src-element"
	schemaElementReferenceBlockXSD11SpecRef     = "xsd11-structures#anchor8458"
	schemaElementReferenceDuplicateXSD10SpecRef = "xsd10-structures#coss-particle"
	schemaElementReferenceDuplicateXSD11SpecRef = "xsd11-structures#coss-particle"
	schemaElementReferenceImportXSD10SpecRef    = "xsd10-structures#composition-importLicenseReferences"
	schemaElementReferenceImportXSD11SpecRef    = "xsd11-structures#composition-importLicenseReferences"
	schemaAttributeTypeXSD10SpecRef             = "xsd10-structures#Attribute_Declaration_details"
	schemaAttributeTypeXSD11SpecRef             = "xsd11-structures#Attribute_Declaration_details"
	schemaNotationXSD10SpecRef                  = "xsd10-structures#Notation_Declaration_details"
	schemaNotationXSD11SpecRef                  = "xsd11-structures#Notation_Declaration_details"
	schemaSubstitutionAffiliationXSD10SpecRef   = "xsd10-structures#Element_Declaration_details"
	schemaSubstitutionAffiliationXSD11SpecRef   = "xsd11-structures#ed-substitution_group_affiliations"
	schemaSubstitutionQNameXSD10SpecRef         = "xsd10-structures#src-qname"
	schemaSubstitutionQNameXSD11SpecRef         = "xsd11-structures#src-resolve"
	schemaSubstitutionResolveXSD10SpecRef       = "xsd10-structures#src-resolve"
	schemaSubstitutionResolveXSD11SpecRef       = "xsd11-structures#src-resolve"
	schemaSubstitutionConstraintXSD10SpecRef    = "xsd10-structures#coss-element"
	schemaSubstitutionConstraintXSD11SpecRef    = "xsd11-structures#coss-element"
	schemaBlockComplexXSD10SpecRef              = "xsd10-structures#Complex_Type_Definition_details"
	schemaBlockComplexXSD11SpecRef              = "xsd11-structures#Complex_Type_Definition_details"
	schemaBlockDefaultXSD10SpecRef              = "xsd10-structures#element-schema"
	schemaBlockDefaultXSD11SpecRef              = "xsd11-structures#element-schema"
	schemaAnyAttributeXSD10SpecRef              = "xsd10-structures#element-anyAttribute"
	schemaAnyAttributeXSD11SpecRef              = "xsd11-structures#element-anyAttribute"
	schemaComplexTypeDerivationXSD10SpecRef     = "xsd10-structures#derivation-ok-restriction"
	schemaComplexTypeDerivationXSD11SpecRef     = "xsd11-structures#derivation-ok-restriction"
	schemaComplexContentExtensionXSD10SpecRef   = "xsd10-structures#element-complexContent..extension"
	schemaComplexContentExtensionXSD11SpecRef   = "xsd11-structures#element-complexContent..extension"
	schemaComplexTypeExtensionXSD10SpecRef      = "xsd10-structures#cos-ct-extends"
	schemaComplexTypeExtensionXSD11SpecRef      = "xsd11-structures#cos-ct-extends"
	schemaComplexParticleExtensionXSD10SpecRef  = "xsd10-structures#cos-particle-extend"
	schemaComplexParticleExtensionXSD11SpecRef  = "xsd11-structures#cos-particle-extend"
)

var (
	errSchemaSimpleTypeBaseUnresolved         = errors.New("simple type base is unresolved")
	errSchemaSimpleTypeBaseWrongKind          = errors.New("simple type base has the wrong kind")
	errSchemaSimpleTypeBaseAmbiguous          = errors.New("simple type base is ambiguous")
	errSchemaSimpleTypeBaseCycle              = errors.New("simple type base is cyclic")
	errSchemaSimpleTypeInvalidDerivation      = errors.New("simple type derivation is invalid")
	errSchemaSimpleTypeRestrictionUnsupported = errors.New("simple type restriction variety is not implemented")
	errSchemaElementTypeUnresolved            = errors.New("element type is unresolved")
	errSchemaElementTypeWrongKind             = errors.New("element type has the wrong kind")
	errSchemaElementTypeAmbiguous             = errors.New("element type is ambiguous")
	errSchemaElementReferenceUnresolved       = errors.New("element reference is unresolved")
	errSchemaElementReferenceWrongKind        = errors.New("element reference has the wrong target kind")
	errSchemaElementReferenceAmbiguous        = errors.New("element reference is ambiguous")
	errSchemaElementReferenceNamespace        = errors.New("element reference namespace is not imported")
	errSchemaElementReferenceDuplicate        = errors.New("element reference particle is duplicated")
	errSchemaElementReferenceBlock            = errors.New("element reference cannot specify block")
	errSchemaAttributeTypeUnresolved          = errors.New("attribute type is unresolved")
	errSchemaAttributeTypeWrongKind           = errors.New("attribute type has the wrong kind")
	errSchemaAttributeTypeAmbiguous           = errors.New("attribute type is ambiguous")
	errSchemaAttributeTypeUnsupported         = errors.New("attribute type is unsupported")
	errSchemaGlobalDeclarationDuplicate       = errors.New("global declaration is duplicated")
	errSchemaElementDuplicate                 = errors.New("global element declaration is duplicated")
	errSchemaElementTargetNamespace           = errors.New("local element targetNamespace is not representable in the supported direct-choice model")
	errSchemaPrecisionDecimalVersion          = errors.New("precisionDecimal is unavailable in the selected XSD version policy")
	errSchemaNotationPublic                   = errors.New("notation public identifier is invalid")
	errSchemaNotationSystem                   = errors.New("notation system identifier is invalid")
	errSchemaSubstitutionQName                = errors.New("substitution-group affiliation QName is invalid")
	errSchemaSubstitutionCardinality          = errors.New("substitutionGroup has invalid cardinality")
	errSchemaSubstitutionUnresolved           = errors.New("substitution-group head is unresolved")
	errSchemaSubstitutionWrongKind            = errors.New("substitution-group head has the wrong kind")
	errSchemaSubstitutionAmbiguous            = errors.New("substitution-group head is ambiguous")
	errSchemaSubstitutionSelf                 = errors.New("element affiliates with itself")
	errSchemaSubstitutionImport               = errors.New("substitution-group head namespace is not directly imported")
	errSchemaSubstitutionTypeInvalid          = errors.New("substitution-group type derivation is invalid")
	errSchemaSubstitutionTypeUnsupported      = errors.New("substitution-group type derivation is not implemented")
	errSchemaSubstitutionCycle                = errors.New("substitution-group affiliations form a cycle")
	errSchemaBlock                            = errors.New("schema block value is invalid")
	errSchemaAnyAttributeUnsupported          = errors.New("anyAttribute wildcard is not implemented")
	errSchemaComplexTypeBaseUnresolved        = errors.New("complex type base is unresolved")
	errSchemaComplexTypeBaseWrongKind         = errors.New("complex type base has the wrong kind")
	errSchemaComplexTypeBaseAmbiguous         = errors.New("complex type base is ambiguous")
	errSchemaComplexTypeBaseUnsupported       = errors.New("complex type named base is not implemented")
	errSchemaComplexTypeBaseRequired          = errors.New("complex type base is required")
	errSchemaComplexTypeBaseCycle             = errors.New("complex type bases form a cycle")
	errSchemaComplexTypeBaseNonEmpty          = errors.New("complex type base has nonempty content")
	errLanguagePolicyMismatch                 = errors.New("recognized XSD 1.1 behavior is outside the selected XSD 1.0 policy")
)

const schemaInstanceNamespaceURI = "http://www.w3.org/2001/XMLSchema-instance"

type schemaTargetNamespace struct {
	value   string
	present bool
	loc     Loc
}

type schemaDocumentFacts struct {
	targetNamespace             schemaTargetNamespace
	elementFormDefaultQualified bool
	blockDefault                schemaBlockPolicy
	chameleon                   bool
}

type schemaBlockSet uint8

const (
	schemaBlockExtension schemaBlockSet = 1 << iota
	schemaBlockRestriction
	schemaBlockSubstitution
	schemaBlockAll = schemaBlockExtension | schemaBlockRestriction | schemaBlockSubstitution
)

const (
	schemaBlockElementMask = schemaBlockAll
	schemaBlockComplexMask = schemaBlockExtension | schemaBlockRestriction
)

type schemaBlockPolicy struct {
	set     schemaBlockSet
	loc     Loc
	present bool
}

var schemaBlockValueOrder = [...]struct {
	bit   schemaBlockSet
	value string
}{
	{bit: schemaBlockExtension, value: "extension"},
	{bit: schemaBlockRestriction, value: "restriction"},
	{bit: schemaBlockSubstitution, value: "substitution"},
}

func (set schemaBlockSet) values() []string {
	if set == 0 {
		return nil
	}
	values := make([]string, 0, len(schemaBlockValueOrder))
	for _, item := range schemaBlockValueOrder {
		if set&item.bit == 0 {
			continue
		}
		values = append(values, item.value)
	}
	return values
}

func (policy schemaBlockPolicy) project(mask schemaBlockSet) schemaBlockPolicy {
	policy.set &= mask
	return policy
}

type schemaBlockPolicyScope uint8

const (
	schemaBlockDocumentDefault schemaBlockPolicyScope = iota + 1
	schemaBlockElement
	schemaBlockComplex
)

func schemaBlockSpecRef(version XSDVersion, scope schemaBlockPolicyScope) string {
	switch scope {
	case schemaBlockDocumentDefault:
		if version == XSDVersion10 {
			return schemaBlockDefaultXSD10SpecRef
		}
		if version == XSDVersion11 {
			return schemaBlockDefaultXSD11SpecRef
		}
	case schemaBlockElement:
		if version == XSDVersion10 {
			return schemaElementTypeXSD10SpecRef
		}
		if version == XSDVersion11 {
			return schemaElementTypeXSD11SpecRef
		}
	case schemaBlockComplex:
		if version == XSDVersion10 {
			return schemaBlockComplexXSD10SpecRef
		}
		if version == XSDVersion11 {
			return schemaBlockComplexXSD11SpecRef
		}
	}
	return ""
}

func newSchemaBlockDiagnostic(attribute syntaxAttribute, message string, version XSDVersion, scope schemaBlockPolicyScope, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaBlockCode,
		loc:     attribute.loc,
		message: message,
		specRef: schemaBlockSpecRef(version, scope),
		cause:   cause,
	}
}

func schemaBlockValueDiagnostic(attribute syntaxAttribute, message string, version XSDVersion, scope schemaBlockPolicyScope) Diagnostic {
	return newSchemaBlockDiagnostic(
		attribute,
		message,
		version,
		scope,
		fmt.Errorf("%w: %s", errSchemaBlock, message),
	)
}

func schemaBlockPolicyFromAttribute(attribute syntaxAttribute, allowed schemaBlockSet, version XSDVersion, scope schemaBlockPolicyScope) (schemaBlockPolicy, error) {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		return schemaBlockPolicy{loc: attribute.loc, present: true}, nil
	}
	tokens := strings.Split(lexeme, " ")
	if len(tokens) != 1 {
		for _, token := range tokens {
			if token != "#all" {
				continue
			}
			return schemaBlockPolicy{}, schemaBlockValueDiagnostic(
				attribute,
				fmt.Sprintf("attribute %q cannot combine #all with other values", attribute.name.local),
				version,
				scope,
			)
		}
	}
	if len(tokens) == 1 && tokens[0] == "#all" {
		return schemaBlockPolicy{set: allowed, loc: attribute.loc, present: true}, nil
	}
	var set schemaBlockSet
	for _, token := range tokens {
		var bit schemaBlockSet
		switch token {
		case "extension":
			bit = schemaBlockExtension
		case "restriction":
			bit = schemaBlockRestriction
		case "substitution":
			bit = schemaBlockSubstitution
		default:
			return schemaBlockPolicy{}, schemaBlockValueDiagnostic(
				attribute,
				fmt.Sprintf("attribute %q has an invalid block value %q", attribute.name.local, token),
				version,
				scope,
			)
		}
		if allowed&bit == 0 {
			return schemaBlockPolicy{}, schemaBlockValueDiagnostic(
				attribute,
				fmt.Sprintf("attribute %q has an invalid block value %q", attribute.name.local, token),
				version,
				scope,
			)
		}
		set |= bit
	}
	return schemaBlockPolicy{set: set, loc: attribute.loc, present: true}, nil
}

func syntaxDocumentBlockDefaultPolicy(document *syntaxDocument, version XSDVersion) (schemaBlockPolicy, error) {
	if document == nil || document.root == nil {
		return schemaBlockPolicy{}, newSchemaBridgeInvariant(Loc{}, "schema document has no root while reading blockDefault")
	}
	attributes := syntaxAttributesByLocal(document.root, "blockDefault")
	if len(attributes) == 0 {
		return schemaBlockPolicy{}, nil
	}
	if len(attributes) != 1 {
		return schemaBlockPolicy{}, schemaBlockValueDiagnostic(
			attributes[1],
			"schema root attribute \"blockDefault\" must be unique",
			version,
			schemaBlockDocumentDefault,
		)
	}
	return schemaBlockPolicyFromAttribute(attributes[0], schemaBlockAll, version, schemaBlockDocumentDefault)
}

func schemaDeclarationBlockPolicy(element *syntaxElement, defaultPolicy schemaBlockPolicy, allowed schemaBlockSet, version XSDVersion, scope schemaBlockPolicyScope) (schemaBlockPolicy, bool, error) {
	attributes := syntaxAttributesByLocal(element, "block")
	if len(attributes) > 1 {
		return schemaBlockPolicy{}, true, schemaBlockValueDiagnostic(
			attributes[1],
			element.name.local+" attribute \"block\" must be unique",
			version,
			scope,
		)
	}
	if len(attributes) == 1 {
		policy, err := schemaBlockPolicyFromAttribute(attributes[0], allowed, version, scope)
		return policy, true, err
	}
	return defaultPolicy.project(allowed), false, nil
}

// discoverSchema completes the internal pipeline used by ParseSchema.
func discoverSchema(root ResolvedSource, resolver Resolver) (Schema, error) {
	return discoverSchemaWithPolicy(root, resolver, Compatibility)
}

func discoverSchemaWithPolicy(root ResolvedSource, resolver Resolver, policy LanguagePolicy) (Schema, error) {
	discovery, err := discoverSyntaxWithPolicy(root, resolver, policy)
	if err != nil {
		return Schema{}, err
	}
	return newSchemaFromDiscoveryWithPolicy(discovery, policy)
}

func newSchemaFromDiscovery(discovery syntaxDiscoveryResult) (Schema, error) {
	return newSchemaFromDiscoveryWithPolicy(discovery, Compatibility)
}

func newSchemaFromDiscoveryWithPolicy(discovery syntaxDiscoveryResult, policy LanguagePolicy) (Schema, error) {
	declaredNamespaces, sourceIndices, err := schemaDiscoveryNamespacesWithPolicy(discovery.documents, policy)
	if err != nil {
		return Schema{}, err
	}

	namespaces, err := schemaEffectiveTargetNamespaces(discovery.edges, sourceIndices, declaredNamespaces)
	if err != nil {
		return Schema{}, err
	}
	err = validateSchemaComposition(discovery.edges, sourceIndices, declaredNamespaces, namespaces)
	if err != nil {
		return Schema{}, err
	}
	visibleSources, err := schemaDocumentVisibleSources(discovery.documents, discovery.edges, sourceIndices)
	if err != nil {
		return Schema{}, err
	}

	version, err := xsdVersionForLanguagePolicy(policy)
	if err != nil {
		return Schema{}, invalidLanguagePolicyDiagnostic(policy, err)
	}
	inputs, err := schemaDocumentInputs(discovery.documents, namespaces, visibleSources, version)
	if err != nil {
		return Schema{}, err
	}

	schema, err := newSchemaWithPolicyAndEdges(inputs, discovery.edges, policy)
	if err != nil {
		return Schema{}, err
	}
	return schema, nil
}

func schemaDiscoveryNamespacesWithPolicy(documents []*syntaxDocument, policy LanguagePolicy) ([]schemaTargetNamespace, map[SourceID]int, error) {
	err := validateLanguagePolicy(policy)
	if err != nil {
		return nil, nil, invalidLanguagePolicyDiagnostic(policy, err)
	}
	namespaces := make([]schemaTargetNamespace, len(documents))
	sourceIndices := make(map[SourceID]int, len(documents))
	for index, document := range documents {
		if err := validateDiscoveredDocument(document, sourceIndices, policy); err != nil {
			return nil, nil, err
		}
		namespace, err := syntaxDocumentTargetNamespace(document)
		if err != nil {
			return nil, nil, err
		}
		sourceIndices[document.source] = index
		namespaces[index] = namespace
	}
	return namespaces, sourceIndices, nil
}

func validateDiscoveredDocument(document *syntaxDocument, sourceIndices map[SourceID]int, policy LanguagePolicy) error {
	if document == nil || document.root == nil {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxDocumentNoRootCode,
			Loc{},
			"schema discovery result contains a document without a root",
			nil,
		)
	}
	if document.source == "" {
		return newDiagnostic(
			FailureInternal,
			diagnosticSchemaEmptySourceCode,
			document.root.loc,
			"schema discovery result contains an empty source identity",
			nil,
		)
	}
	if _, exists := sourceIndices[document.source]; exists {
		return newDiagnostic(
			FailureInternal,
			diagnosticSchemaRepeatedSourceCode,
			document.root.loc,
			fmt.Sprintf("schema discovery result repeats source %q", document.source),
			nil,
		)
	}
	return validateSyntaxDocumentStructureWithPolicy(document, policy)
}

func schemaDocumentInputs(documents []*syntaxDocument, namespaces []schemaTargetNamespace, visibleSources map[SourceID][]SourceID, version XSDVersion) ([]schemaDocumentInput, error) {
	inputs := make([]schemaDocumentInput, 0, len(documents))
	for index, document := range documents {
		input, err := schemaDocumentInputAt(document, index, namespaces, visibleSources, version)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func schemaDocumentInputAt(
	document *syntaxDocument,
	index int,
	namespaces []schemaTargetNamespace,
	visibleSources map[SourceID][]SourceID,
	version XSDVersion,
) (schemaDocumentInput, error) {
	if document == nil {
		return schemaDocumentInput{}, newSchemaBridgeInvariant(Loc{}, "schema document input has no root")
	}
	if document.root == nil {
		return schemaDocumentInput{}, newSchemaBridgeInvariant(Loc{}, "schema document input has no root")
	}
	if index >= len(namespaces) {
		return schemaDocumentInput{}, newSchemaBridgeInvariant(document.root.loc, "schema document input has no target namespace")
	}
	declaredNamespace, err := syntaxDocumentTargetNamespace(document)
	if err != nil {
		return schemaDocumentInput{}, err
	}
	elementFormDefaultQualified, err := syntaxDocumentElementFormDefault(document)
	if err != nil {
		return schemaDocumentInput{}, err
	}
	blockDefault, err := syntaxDocumentBlockDefaultPolicy(document, version)
	if err != nil {
		return schemaDocumentInput{}, err
	}
	facts := schemaDocumentFacts{
		targetNamespace:             namespaces[index],
		elementFormDefaultQualified: elementFormDefaultQualified,
		blockDefault:                blockDefault,
		chameleon:                   !declaredNamespace.present && namespaces[index].present,
	}
	declarations, err := schemaDocumentDeclarationsWithFacts(document, facts, version)
	if err != nil {
		return schemaDocumentInput{}, err
	}
	input := schemaDocumentInput{
		source:          document.source,
		rootLoc:         document.root.loc,
		targetNamespace: namespaces[index].value,
		declarations:    declarations,
	}
	if visibleSources == nil {
		return input, nil
	}
	visible, ok := visibleSources[document.source]
	if !ok {
		return schemaDocumentInput{}, newSchemaBridgeInvariant(document.root.loc, "schema document input has no visibility entry")
	}
	input.visibleSources = append([]SourceID(nil), visible...)
	return input, nil
}

func syntaxDocumentElementFormDefault(document *syntaxDocument) (bool, error) {
	if document == nil || document.root == nil {
		return false, newSchemaBridgeInvariant(Loc{}, "schema document has no root while reading elementFormDefault")
	}
	attributes := syntaxAttributesByLocal(document.root, "elementFormDefault")
	if len(attributes) == 0 {
		return false, nil
	}
	if len(attributes) != 1 {
		return false, newSchemaCompositionDiagnostic(attributes[1].loc, "attribute \"elementFormDefault\" must be unique")
	}
	if err := validateSchemaEnum(attributes[0], "qualified", "unqualified"); err != nil {
		return false, err
	}
	return collapseXMLWhitespace(attributes[0].value) == "qualified", nil
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
	value := collapseXMLWhitespace(attribute.value)
	if value == "" {
		return schemaTargetNamespace{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaTargetNamespaceCode,
			attribute.loc,
			"schema targetNamespace cannot be empty when present",
			nil,
		)
	}
	return schemaTargetNamespace{
		value:   value,
		present: true,
		loc:     attribute.loc,
	}, nil
}

func schemaEffectiveTargetNamespaces(
	edges []syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
	declared []schemaTargetNamespace,
) ([]schemaTargetNamespace, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	effective := append([]schemaTargetNamespace(nil), declared...)
	for changed := true; changed; {
		changed = false
		for _, edge := range edges {
			if edge.kind != syntaxReferenceInclude {
				continue
			}
			applied, err := schemaApplyChameleonInclude(edge, sourceIndices, declared, effective)
			if err != nil {
				return nil, err
			}
			changed = changed || applied
		}
	}
	return effective, nil
}

func schemaApplyChameleonInclude(
	edge syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
	declared []schemaTargetNamespace,
	effective []schemaTargetNamespace,
) (bool, error) {
	sourceIndex, targetIndex, err := schemaCompositionEdgeIndices(edge, sourceIndices, len(declared))
	if err != nil {
		return false, err
	}
	if declared[targetIndex].present || !effective[sourceIndex].present {
		return false, nil
	}
	if effective[targetIndex].present {
		if effective[targetIndex].value != effective[sourceIndex].value {
			return false, newSchemaCompositionDiagnostic(
				edge.loc,
				fmt.Sprintf("chameleon include target namespace %q conflicts with including namespace %q", effective[targetIndex].value, effective[sourceIndex].value),
			)
		}
		return false, nil
	}
	effective[targetIndex] = schemaTargetNamespace{
		value:   effective[sourceIndex].value,
		present: true,
		loc:     effective[sourceIndex].loc,
	}
	return true, nil
}

func validateSchemaComposition(
	edges []syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
	declared []schemaTargetNamespace,
	effective []schemaTargetNamespace,
) error {
	for _, edge := range edges {
		if err := validateSchemaCompositionEdge(edge, sourceIndices, declared, effective); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaCompositionEdge(
	edge syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
	declared []schemaTargetNamespace,
	effective []schemaTargetNamespace,
) error {
	sourceIndex, targetIndex, err := schemaCompositionEdgeIndices(edge, sourceIndices, len(declared))
	if err != nil {
		return err
	}
	if len(effective) != len(declared) {
		return newSchemaBridgeInvariant(edge.loc, "schema composition namespace tables have different lengths")
	}
	parent := effective[sourceIndex]
	child := declared[targetIndex]
	switch edge.kind {
	case syntaxReferenceInclude:
		return validateSchemaInclude(edge, parent, child, effective[targetIndex])
	case syntaxReferenceImport:
		return validateSchemaImport(edge, parent, child)
	default:
		return newSchemaBridgeInvariant(edge.loc, "schema edge has an unknown reference kind")
	}
}

func schemaCompositionEdgeIndices(edge syntaxDocumentEdge, sourceIndices map[SourceID]int, namespaceCount int) (int, int, error) {
	sourceIndex, ok := sourceIndices[edge.source]
	if !ok {
		return 0, 0, newSchemaBridgeInvariant(edge.loc, fmt.Sprintf("schema edge source %q is unknown", edge.source))
	}
	targetIndex, ok := sourceIndices[edge.target]
	if !ok {
		return 0, 0, newSchemaBridgeInvariant(edge.loc, fmt.Sprintf("schema edge target %q is unknown", edge.target))
	}
	if sourceIndex < 0 || targetIndex < 0 || sourceIndex >= namespaceCount || targetIndex >= namespaceCount {
		return 0, 0, newSchemaBridgeInvariant(edge.loc, "schema edge points outside the namespace table")
	}
	return sourceIndex, targetIndex, nil
}

func validateSchemaInclude(
	edge syntaxDocumentEdge,
	parent schemaTargetNamespace,
	declaredChild schemaTargetNamespace,
	effectiveChild schemaTargetNamespace,
) error {
	if !parent.present && declaredChild.present {
		return newSchemaCompositionDiagnostic(edge.loc, "schema include adds a target namespace to a no-namespace document")
	}
	if declaredChild.present && parent.present && parent.value != declaredChild.value {
		return newSchemaCompositionDiagnostic(
			edge.loc,
			fmt.Sprintf("schema include target namespace %q does not match including namespace %q", declaredChild.value, parent.value),
		)
	}
	if parent.present && !effectiveChild.present {
		return newSchemaBridgeInvariant(edge.loc, "chameleon include has no effective target namespace")
	}
	if parent.present && effectiveChild.value != parent.value {
		return newSchemaCompositionDiagnostic(
			edge.loc,
			fmt.Sprintf("chameleon include target namespace %q does not match including namespace %q", effectiveChild.value, parent.value),
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

func schemaDocumentVisibleSources(
	documents []*syntaxDocument,
	edges []syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
) (map[SourceID][]SourceID, error) {
	visible := make(map[SourceID][]SourceID, len(documents))
	if len(documents) == 0 {
		return visible, nil
	}
	parents, err := schemaVisibilityParents(documents, edges, sourceIndices)
	if err != nil {
		return nil, err
	}
	members := schemaVisibilityMembers(documents, parents)
	for index, document := range documents {
		available, err := schemaDocumentVisibleSourcesAt(index, members, parents, edges, sourceIndices)
		if err != nil {
			return nil, err
		}
		visible[document.source] = available
	}
	return visible, nil
}

func schemaVisibilityParents(
	documents []*syntaxDocument,
	edges []syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
) ([]int, error) {
	parents := make([]int, len(documents))
	for index, document := range documents {
		if document == nil {
			return nil, newSchemaBridgeInvariant(Loc{}, "schema visibility contains a document without a root")
		}
		if document.root == nil {
			return nil, newSchemaBridgeInvariant(Loc{}, "schema visibility contains a document without a root")
		}
		parents[index] = index
	}
	for _, edge := range edges {
		if edge.kind != syntaxReferenceInclude {
			continue
		}
		sourceIndex, targetIndex, err := schemaCompositionEdgeIndices(edge, sourceIndices, len(documents))
		if err != nil {
			return nil, err
		}
		schemaVisibilityUnion(parents, sourceIndex, targetIndex)
	}
	return parents, nil
}

func schemaVisibilityMembers(documents []*syntaxDocument, parents []int) [][]SourceID {
	members := make([][]SourceID, len(documents))
	for index, document := range documents {
		root := schemaVisibilityFind(parents, index)
		members[root] = append(members[root], document.source)
	}
	return members
}

func schemaDocumentVisibleSourcesAt(
	index int,
	members [][]SourceID,
	parents []int,
	edges []syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
) ([]SourceID, error) {
	root := schemaVisibilityFind(parents, index)
	available := append([]SourceID(nil), members[root]...)
	for _, edge := range edges {
		if edge.kind != syntaxReferenceImport {
			continue
		}
		sourceIndex, targetIndex, err := schemaCompositionEdgeIndices(edge, sourceIndices, len(parents))
		if err != nil {
			return nil, err
		}
		if sourceIndex != index {
			continue
		}
		targetRoot := schemaVisibilityFind(parents, targetIndex)
		for _, source := range members[targetRoot] {
			if sourceIDInList(available, source) {
				continue
			}
			available = append(available, source)
		}
	}
	return available, nil
}

func schemaVisibilityFind(parents []int, index int) int {
	for parents[index] != index {
		parents[index] = parents[parents[index]]
		index = parents[index]
	}
	return index
}

func schemaVisibilityUnion(parents []int, left, right int) {
	leftRoot := schemaVisibilityFind(parents, left)
	rightRoot := schemaVisibilityFind(parents, right)
	if leftRoot == rightRoot {
		return
	}
	if leftRoot < rightRoot {
		parents[rightRoot] = leftRoot
		return
	}
	parents[leftRoot] = rightRoot
}

func sourceIDInList(sources []SourceID, wanted SourceID) bool {
	for _, source := range sources {
		if source == wanted {
			return true
		}
	}
	return false
}

func schemaVisibleCandidates(
	candidates []int,
	referringSource SourceID,
	records []schemaComponentRecord,
	visibleSources map[SourceID][]SourceID,
	loc Loc,
) ([]int, error) {
	available, ok := visibleSources[referringSource]
	if !ok {
		return nil, newSchemaBridgeInvariant(
			loc,
			fmt.Sprintf("schema document %q has no visibility entry", referringSource),
		)
	}
	visible := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate < 0 || candidate >= len(records) {
			return nil, newSchemaBridgeInvariant(loc, "schema type lookup has an invalid record index")
		}
		if !sourceIDInList(available, records[candidate].id.Source()) {
			continue
		}
		visible = append(visible, candidate)
	}
	return visible, nil
}

func schemaDocumentDeclarationsWithFacts(document *syntaxDocument, facts schemaDocumentFacts, version XSDVersion) ([]schemaComponentInput, error) {
	declarations := make([]schemaComponentInput, 0)
	for _, node := range document.root.children {
		declaration, present, err := schemaDocumentDeclarationWithFacts(node, facts, version)
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

func schemaDocumentDeclaration(node syntaxNode, targetNamespace string, version XSDVersion) (schemaComponentInput, bool, error) {
	facts := schemaDocumentFacts{
		targetNamespace: schemaTargetNamespace{value: targetNamespace, present: targetNamespace != ""},
	}
	return schemaDocumentDeclarationWithFacts(node, facts, version)
}

func schemaDocumentDeclarationWithFacts(node syntaxNode, facts schemaDocumentFacts, version XSDVersion) (schemaComponentInput, bool, error) {
	element, kind, present, err := schemaDocumentDeclarationSyntax(node)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	if !present {
		return schemaComponentInput{}, false, nil
	}
	declaration, err := schemaDocumentDeclarationInput(element, kind, facts, version)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	return declaration, true, nil
}

func schemaDocumentDeclarationSyntax(node syntaxNode) (*syntaxElement, ComponentKind, bool, error) {
	textNode, ok := node.(syntaxText)
	if ok {
		if xmlWhitespace([]byte(textNode.data)) {
			return nil, "", false, nil
		}
		return nil, "", false, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			textNode.loc,
			"schema root contains non-whitespace character data",
			nil,
		)
	}
	element, ok := node.(*syntaxElement)
	if !ok {
		return nil, "", false, newSchemaBridgeInvariant(Loc{}, "schema root contains an unknown syntax node")
	}
	if element.name.namespace != xsdNamespaceURI {
		return nil, "", false, newSchemaSyntaxUnsupported(element.loc, "schema root contains an unsupported non-XSD construct")
	}
	kind, named := schemaDeclarationKind(element.name.local)
	if !named {
		ignored, err := schemaRootChildIgnored(element)
		if err != nil {
			return nil, "", false, err
		}
		if ignored {
			return nil, "", false, nil
		}
		return nil, "", false, newSchemaSyntaxUnsupported(
			element.loc,
			fmt.Sprintf("XSD schema child <%s> is not implemented", element.name.local),
		)
	}
	return element, kind, true, nil
}

func schemaDocumentDeclarationInput(element *syntaxElement, kind ComponentKind, facts schemaDocumentFacts, version XSDVersion) (schemaComponentInput, error) { //nolint:gocognit // Keep declaration-kind dispatch and phase-local input construction together.
	name, err := schemaDeclarationName(element, facts.targetNamespace.value)
	if err != nil {
		return schemaComponentInput{}, err
	}
	declaration := schemaComponentInput{
		kind: kind,
		name: name,
		loc:  element.loc,
	}
	if kind == ComponentKindElementDeclaration {
		elementType, elementErr := schemaElementTypeInput(element, facts, version)
		if elementErr != nil {
			return schemaComponentInput{}, elementErr
		}
		declaration.element = elementType
	}
	if kind == ComponentKindAttributeDeclaration {
		attributeType, attributeErr := schemaAttributeTypeInput(element, version)
		if attributeErr != nil {
			return schemaComponentInput{}, attributeErr
		}
		declaration.attribute = attributeType
	}
	if kind == ComponentKindNotationDeclaration {
		notation, notationErr := schemaNotationInputFromElement(element, version)
		if notationErr != nil {
			return schemaComponentInput{}, notationErr
		}
		declaration.notation = notation
	}
	if kind == ComponentKindComplexTypeDefinition {
		complexType, complexErr := schemaComplexTypeInputFromElementWithFacts(element, facts, version)
		if complexErr != nil {
			return schemaComponentInput{}, complexErr
		}
		declaration.complexType = complexType
	}
	if kind == ComponentKindModelGroupDefinition {
		modelGroup, groupErr := schemaModelGroupInputFromElementWithFacts(element, facts, version)
		if groupErr != nil {
			return schemaComponentInput{}, groupErr
		}
		declaration.modelGroup = modelGroup
	}
	if kind != ComponentKindSimpleTypeDefinition {
		return declaration, nil
	}
	simpleType, err := schemaSimpleTypeInputFromElement(element)
	if err != nil {
		return schemaComponentInput{}, err
	}
	declaration.simpleType = simpleType
	return declaration, nil
}

func schemaAttributeTypeInput(element *syntaxElement, version XSDVersion) (*schemaAttributeInput, error) {
	if element == nil {
		return nil, newSchemaBridgeInvariant(Loc{}, "construct attribute type input from a nil element")
	}
	attributes := syntaxAttributesByLocal(element, "type")
	inline := inlineSimpleTypeChild(element)
	if len(attributes) > 1 {
		return nil, newSchemaCompositionDiagnostic(attributes[1].loc, "attribute type attribute must be unique")
	}
	if len(attributes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "attribute cannot combine type attribute with an inline simpleType")
	}
	if len(attributes) == 0 {
		if inline == nil {
			return nil, nil
		}
		if err := validateInlineSchemaType(inline, version); err != nil {
			return nil, err
		}
		return nil, newSchemaSyntaxUnsupportedForVersion(
			inline.loc,
			"inline anonymous simple types in global attributes are not implemented",
			version,
		)
	}
	declaredType, err := expandSchemaQName(element, attributes[0])
	if err != nil {
		return nil, err
	}
	return &schemaAttributeInput{
		declaredType: declaredType,
		typeLoc:      attributes[0].loc,
	}, nil
}

func schemaElementTypeInput(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion) (*schemaElementInput, error) {
	abstract, abstractPresent, abstractLoc, err := schemaElementBooleanAttribute(element, "abstract")
	if err != nil {
		return nil, err
	}
	nillable, nillablePresent, nillableLoc, err := schemaElementBooleanAttribute(element, "nillable")
	if err != nil {
		return nil, err
	}
	substitutionGroup, err := schemaElementSubstitutionGroupInputs(element, version)
	if err != nil {
		return nil, err
	}
	block, explicitBlock, err := schemaDeclarationBlockPolicy(
		element,
		facts.blockDefault,
		schemaBlockElementMask,
		version,
		schemaBlockElement,
	)
	if err != nil {
		return nil, err
	}
	attributes := syntaxAttributesByLocal(element, "type")
	inline := inlineSimpleTypeChild(element)
	if len(attributes) > 1 {
		return nil, newSchemaCompositionDiagnostic(element.loc, "element type attribute must be unique")
	}
	if len(attributes) == 0 && inline == nil {
		return schemaElementTypeInputWithoutDeclaredType(
			element,
			version,
			abstractPresent,
			abstractLoc,
			nillablePresent,
			nillableLoc,
			substitutionGroup,
			block,
			explicitBlock,
		)
	}
	if len(attributes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "element cannot combine type attribute with an inline simpleType")
	}
	if inline != nil {
		return schemaElementTypeInputForInline(inline, version, abstract, nillable, substitutionGroup, block)
	}
	declaredType, err := expandSchemaQName(element, attributes[0])
	if err != nil {
		return nil, err
	}
	return &schemaElementInput{
		declaredType:      declaredType,
		typeLoc:           attributes[0].loc,
		abstract:          abstract,
		nillable:          nillable,
		block:             block,
		substitutionGroup: substitutionGroup,
	}, nil
}

func schemaElementTypeInputWithoutDeclaredType(
	element *syntaxElement,
	version XSDVersion,
	abstractPresent bool,
	abstractLoc Loc,
	nillablePresent bool,
	nillableLoc Loc,
	substitutionGroup []schemaElementSubstitutionGroupInput,
	block schemaBlockPolicy,
	explicitBlock bool,
) (*schemaElementInput, error) {
	if noTypeErr := validateSchemaElementWithoutDeclaredType(element, version, abstractPresent, abstractLoc, nillablePresent, nillableLoc); noTypeErr != nil {
		return nil, noTypeErr
	}
	if explicitBlock || block.set != 0 {
		loc := block.loc
		if loc.IsZero() {
			loc = element.loc
		}
		return nil, newSchemaSyntaxUnsupportedForVersion(
			loc,
			"block facts for global elements without a supported declared type are not implemented",
			version,
		)
	}
	if len(substitutionGroup) == 0 {
		return nil, nil
	}
	return nil, newSchemaSyntaxUnsupportedForVersion(
		substitutionGroup[0].loc,
		"substitutionGroup affiliations for global elements without a declared type are not implemented",
		version,
	)
}

func schemaElementTypeInputForInline(
	inline *syntaxElement,
	version XSDVersion,
	abstract bool,
	nillable bool,
	substitutionGroup []schemaElementSubstitutionGroupInput,
	block schemaBlockPolicy,
) (*schemaElementInput, error) {
	if len(substitutionGroup) > 0 {
		return nil, newSchemaSyntaxUnsupportedForVersion(
			substitutionGroup[0].loc,
			"substitutionGroup affiliations for inline global elements are not implemented",
			version,
		)
	}
	if inlineErr := validateInlineSchemaType(inline, version); inlineErr != nil {
		return nil, inlineErr
	}
	simpleType, simpleTypeErr := schemaSimpleTypeInputFromElement(inline)
	if simpleTypeErr != nil {
		return nil, simpleTypeErr
	}
	return &schemaElementInput{
		typeLoc:          inline.loc,
		inlineSimpleType: simpleType,
		abstract:         abstract,
		nillable:         nillable,
		block:            block,
	}, nil
}

func schemaElementSubstitutionGroupInputs(element *syntaxElement, version XSDVersion) ([]schemaElementSubstitutionGroupInput, error) {
	attributes := syntaxAttributesByLocal(element, "substitutionGroup")
	if len(attributes) == 0 {
		return nil, nil
	}
	if len(attributes) != 1 {
		return nil, newSchemaSubstitutionGroupCardinalityDiagnostic(
			element.loc,
			"substitutionGroup attribute must be unique",
			version,
			fmt.Errorf("%w: %d attributes", errSchemaSubstitutionCardinality, len(attributes)),
		)
	}
	attribute := attributes[0]
	items, err := schemaElementSubstitutionGroupTokens(attribute, version)
	if err != nil {
		return nil, err
	}
	inputs := make([]schemaElementSubstitutionGroupInput, 0, len(items))
	seen := make(map[QName]struct{}, len(items))
	for _, item := range items {
		name, err := expandSchemaSubstitutionGroupQName(element, attribute, item, version)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		inputs = append(inputs, schemaElementSubstitutionGroupInput{name: name, loc: attribute.loc})
	}
	return inputs, nil
}

func validateSchemaElementWithoutDeclaredType(element *syntaxElement, version XSDVersion, abstractPresent bool, abstractLoc Loc, nillablePresent bool, nillableLoc Loc) error {
	if abstractPresent {
		return newSchemaSyntaxUnsupported(
			abstractLoc,
			"global element abstract is not implemented without a declared type",
		)
	}
	if nillablePresent {
		return newSchemaSyntaxUnsupported(
			nillableLoc,
			"global element nillable is not implemented without a declared type",
		)
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "simpleType" {
			continue
		}
		if inlineErr := validateInlineSchemaType(child, version); inlineErr != nil {
			return inlineErr
		}
		return newSchemaSyntaxUnsupportedForVersion(
			child.loc,
			"inline anonymous simple types in global elements are not implemented",
			version,
		)
	}
	return nil
}

func schemaElementBooleanAttribute(element *syntaxElement, local string) (bool, bool, Loc, error) {
	attributes := syntaxAttributesByLocal(element, local)
	if len(attributes) == 0 {
		return false, false, Loc{}, nil
	}
	if len(attributes) != 1 {
		return false, true, element.loc, newSchemaCompositionDiagnostic(
			element.loc,
			fmt.Sprintf("element %s attribute %q must be unique", element.name.local, local),
		)
	}
	value, err := schemaBooleanValue(attributes[0])
	return value, true, attributes[0].loc, err
}

func schemaComplexTypeInputFromElementWithFacts(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion) (*schemaComplexTypeInput, error) {
	block, explicitBlock, err := schemaDeclarationBlockPolicy(
		element,
		facts.blockDefault,
		schemaBlockComplexMask,
		version,
		schemaBlockComplex,
	)
	if err != nil {
		return nil, err
	}
	complexContent := schemaComplexContentChild(element)
	if complexContent != nil {
		if schemaComplexContentExtensionChild(complexContent) != nil {
			return schemaComplexTypeExtensionInput(complexContent, facts, version, block)
		}
		return schemaComplexTypeRestrictionInput(complexContent, block)
	}
	model := schemaComplexTypeModel(element)
	if model == nil {
		if explicitBlock || block.set != 0 {
			loc := block.loc
			if loc.IsZero() {
				loc = element.loc
			}
			return nil, newSchemaSyntaxUnsupportedForVersion(
				loc,
				"block facts for global complex types without a supported particle model are not implemented",
				version,
			)
		}
		return &schemaComplexTypeInput{
			body:                    &schemaComplexTypeEmptyBodyInput{},
			prohibitedSubstitutions: block,
		}, nil
	}

	occurrences, err := schemaParticleOccurrenceRange(model, version)
	if err != nil {
		return nil, err
	}
	anyAttribute, err := schemaDirectAnyAttributeInputFromElement(element)
	if err != nil {
		return nil, err
	}
	if model.name.local == "choice" {
		return schemaChoiceComplexTypeInput(model, occurrences, facts, version, block, anyAttribute)
	}
	return schemaSequenceComplexTypeInput(model, occurrences, facts, version, block, anyAttribute)
}

func schemaComplexTypeRestrictionInput(complexContent *syntaxElement, block schemaBlockPolicy) (*schemaComplexTypeInput, error) {
	restriction := schemaComplexContentRestrictionChild(complexContent)
	if restriction == nil {
		return nil, newSchemaBridgeInvariant(complexContent.loc, "supported complexContent has no restriction child")
	}
	baseAttributes := syntaxAttributesByLocal(restriction, "base")
	if len(baseAttributes) != 1 {
		return nil, newSchemaBridgeInvariant(restriction.loc, "supported complexContent restriction has no unique base")
	}
	base, err := expandSchemaQName(restriction, baseAttributes[0])
	if err != nil {
		return nil, err
	}
	anyAttribute, err := schemaAnyAttributeInputFromElement(restriction)
	if err != nil {
		return nil, err
	}
	if anyAttribute == nil {
		return nil, newSchemaBridgeInvariant(restriction.loc, "supported complexContent restriction has no wildcard")
	}
	return &schemaComplexTypeInput{
		body: &schemaComplexTypeRestrictionBodyInput{
			complexContentLoc: complexContent.loc,
			restrictionLoc:    restriction.loc,
			base: schemaComplexTypeReferenceInput{
				kind: schemaComplexTypeQNameReferenceInput,
				name: base,
				loc:  baseAttributes[0].loc,
			},
			anyAttribute: anyAttribute,
		},
		prohibitedSubstitutions: block,
	}, nil
}

//nolint:gocognit // Keep direct extension particle conversion in source order.
func schemaComplexTypeExtensionInput(complexContent *syntaxElement, facts schemaDocumentFacts, version XSDVersion, block schemaBlockPolicy) (*schemaComplexTypeInput, error) {
	extension := schemaComplexContentExtensionChild(complexContent)
	if extension == nil {
		return nil, newSchemaBridgeInvariant(complexContent.loc, "supported complexContent has no extension child")
	}
	baseAttributes := syntaxAttributesByLocal(extension, "base")
	if len(baseAttributes) != 1 {
		return nil, newSchemaBridgeInvariant(extension.loc, "supported complexContent extension has no unique base")
	}
	base, err := expandSchemaQName(extension, baseAttributes[0])
	if err != nil {
		return nil, err
	}
	model := schemaComplexTypeModel(extension)
	if model == nil {
		return nil, newSchemaBridgeInvariant(extension.loc, "supported complexContent extension has no particle")
	}
	occurrences, err := schemaParticleOccurrenceRange(model, version)
	if err != nil {
		return nil, err
	}
	var particle schemaComplexTypeParticleInput
	switch model.name.local {
	case "choice":
		choice := &schemaChoiceParticleInput{
			loc:          model.loc,
			occurrences:  occurrences,
			alternatives: make([]schemaElementParticleInput, 0),
		}
		for _, node := range model.children {
			child, ok := node.(*syntaxElement)
			if !ok || child.name.local != "element" {
				continue
			}
			alternative, particleErr := schemaElementParticleInputFromElementWithFacts(child, facts, version, true)
			if particleErr != nil {
				return nil, particleErr
			}
			choice.alternatives = append(choice.alternatives, alternative)
		}
		particle = choice
	case "sequence":
		sequence := &schemaSequenceParticleInput{
			loc:         model.loc,
			occurrences: occurrences,
			elements:    make([]schemaElementParticleInput, 0),
		}
		for _, node := range model.children {
			child, ok := node.(*syntaxElement)
			if !ok || child.name.local != "element" {
				continue
			}
			input, particleErr := schemaElementParticleInputFromElementWithFacts(child, facts, version, true)
			if particleErr != nil {
				return nil, particleErr
			}
			sequence.elements = append(sequence.elements, input)
		}
		particle = sequence
	default:
		return nil, newSchemaBridgeInvariant(model.loc, "supported complexContent extension has an unknown particle")
	}
	return &schemaComplexTypeInput{
		body: &schemaComplexTypeExtensionBodyInput{
			complexContentLoc: complexContent.loc,
			extensionLoc:      extension.loc,
			base: schemaComplexTypeReferenceInput{
				kind: schemaComplexTypeQNameReferenceInput,
				name: base,
				loc:  baseAttributes[0].loc,
			},
			particle: particle,
		},
		prohibitedSubstitutions: block,
	}, nil
}

func schemaComplexContentChild(element *syntaxElement) *syntaxElement {
	if element == nil {
		return nil
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "complexContent" {
			continue
		}
		return child
	}
	return nil
}

func schemaComplexContentRestrictionChild(element *syntaxElement) *syntaxElement {
	if element == nil {
		return nil
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "restriction" {
			continue
		}
		return child
	}
	return nil
}

func schemaComplexContentExtensionChild(element *syntaxElement) *syntaxElement {
	if element == nil {
		return nil
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "extension" {
			continue
		}
		return child
	}
	return nil
}

func schemaComplexTypeModel(element *syntaxElement) *syntaxElement {
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "choice" && child.name.local != "sequence" {
			continue
		}
		return child
	}
	return nil
}

func schemaModelGroupModel(element *syntaxElement) *syntaxElement {
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "all" && child.name.local != "choice" && child.name.local != "sequence" {
			continue
		}
		return child
	}
	return nil
}

func schemaModelGroupInputFromElementWithFacts(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion) (*schemaModelGroupInput, error) {
	model := schemaModelGroupModel(element)
	if model == nil {
		return nil, newSchemaBridgeInvariant(element.loc, "model group definition has no model child")
	}
	if model.name.local != "choice" {
		return nil, newSchemaSyntaxUnsupportedForVersion(
			model.loc,
			fmt.Sprintf("named model-group %s particles are not implemented", model.name.local),
			version,
		)
	}
	occurrences, err := schemaParticleOccurrenceRange(model, version)
	if err != nil {
		return nil, err
	}
	choice := &schemaChoiceParticleInput{
		loc:          model.loc,
		occurrences:  occurrences,
		alternatives: make([]schemaElementParticleInput, 0),
	}
	for _, node := range model.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local == "annotation" {
			continue
		}
		if child.name.local != "element" {
			return nil, newSchemaSyntaxUnsupportedForVersion(
				child.loc,
				fmt.Sprintf("named model-group choice child <%s> is not implemented", child.name.local),
				version,
			)
		}
		alternative, err := schemaElementParticleInputFromElementWithFacts(child, facts, version, false)
		if err != nil {
			return nil, err
		}
		if alternative.reference == nil {
			return nil, newSchemaSyntaxUnsupportedForVersion(
				child.loc,
				"named model-group choices require global element references",
				version,
			)
		}
		choice.alternatives = append(choice.alternatives, alternative)
	}
	return &schemaModelGroupInput{particle: choice}, nil
}

func schemaChoiceComplexTypeInput(model *syntaxElement, occurrences particleOccurrenceRange, facts schemaDocumentFacts, version XSDVersion, block schemaBlockPolicy, anyAttribute *schemaAnyAttributeInput) (*schemaComplexTypeInput, error) {
	input := &schemaChoiceParticleInput{
		loc:          model.loc,
		occurrences:  occurrences,
		alternatives: make([]schemaElementParticleInput, 0),
	}
	for _, node := range model.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "element" {
			continue
		}
		alternative, err := schemaElementParticleInputFromElementWithFacts(child, facts, version, true)
		if err != nil {
			return nil, err
		}
		input.alternatives = append(input.alternatives, alternative)
	}
	return &schemaComplexTypeInput{
		body: &schemaComplexTypeDirectBodyInput{
			particle:     input,
			anyAttribute: anyAttribute,
		},
		prohibitedSubstitutions: block,
	}, nil
}

func schemaSequenceComplexTypeInput(model *syntaxElement, occurrences particleOccurrenceRange, facts schemaDocumentFacts, version XSDVersion, block schemaBlockPolicy, anyAttribute *schemaAnyAttributeInput) (*schemaComplexTypeInput, error) {
	input := &schemaSequenceParticleInput{
		loc:         model.loc,
		occurrences: occurrences,
		elements:    make([]schemaElementParticleInput, 0),
	}
	for _, node := range model.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if child.name.local != "element" {
			continue
		}
		alternative, err := schemaElementParticleInputFromElementWithFacts(child, facts, version, true)
		if err != nil {
			return nil, err
		}
		input.elements = append(input.elements, alternative)
	}
	return &schemaComplexTypeInput{
		body: &schemaComplexTypeDirectBodyInput{
			particle:     input,
			anyAttribute: anyAttribute,
		},
		prohibitedSubstitutions: block,
	}, nil
}

func schemaAnyAttributeInputFromElement(element *syntaxElement) (*schemaAnyAttributeInput, error) {
	wildcard, err := schemaAnyAttributeElementFromElement(element)
	if err != nil || wildcard == nil {
		return nil, err
	}
	return schemaExplicitAnyAttributeInputFromElement(wildcard)
}

func schemaDirectAnyAttributeInputFromElement(element *syntaxElement) (*schemaAnyAttributeInput, error) {
	wildcard, err := schemaAnyAttributeElementFromElement(element)
	if err != nil || wildcard == nil {
		return nil, err
	}
	namespaceAttributes := syntaxAttributesByLocal(wildcard, "namespace")
	processContentsAttributes := syntaxAttributesByLocal(wildcard, "processContents")
	if len(namespaceAttributes) == 0 && len(processContentsAttributes) == 0 {
		return &schemaAnyAttributeInput{
			loc:             wildcard.loc,
			namespace:       "##any",
			processContents: "strict",
		}, nil
	}
	return schemaExplicitAnyAttributeInputFromElement(wildcard)
}

func schemaAnyAttributeElementFromElement(element *syntaxElement) (*syntaxElement, error) {
	if element == nil {
		return nil, newSchemaBridgeInvariant(Loc{}, "construct anyAttribute input from a nil element")
	}
	var wildcard *syntaxElement
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "anyAttribute" {
			continue
		}
		if wildcard != nil {
			return nil, newSchemaBridgeInvariant(child.loc, "complex type anyAttribute input is not unique")
		}
		wildcard = child
	}
	if wildcard == nil {
		return nil, nil
	}
	return wildcard, nil
}

func schemaExplicitAnyAttributeInputFromElement(wildcard *syntaxElement) (*schemaAnyAttributeInput, error) {
	namespaceAttributes := syntaxAttributesByLocal(wildcard, "namespace")
	processContentsAttributes := syntaxAttributesByLocal(wildcard, "processContents")
	if len(namespaceAttributes) != 1 || len(processContentsAttributes) != 1 {
		return nil, newSchemaBridgeInvariant(wildcard.loc, "supported anyAttribute input lacks explicit constraints")
	}
	namespace := collapseXMLWhitespace(namespaceAttributes[0].value)
	processContents := collapseXMLWhitespace(processContentsAttributes[0].value)
	if namespace != "##other" || processContents != "lax" {
		return nil, newSchemaBridgeInvariant(wildcard.loc, "unsupported anyAttribute input reached component construction")
	}
	return &schemaAnyAttributeInput{
		loc:                wildcard.loc,
		namespace:          namespace,
		namespaceLoc:       namespaceAttributes[0].loc,
		processContents:    processContents,
		processContentsLoc: processContentsAttributes[0].loc,
	}, nil
}

func schemaElementParticleInputFromElementWithFacts(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion, allowNamespacePolicy bool) (schemaElementParticleInput, error) {
	occurrences, err := schemaParticleOccurrenceRange(element, version)
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	refAttributes := syntaxAttributesByLocal(element, "ref")
	if len(refAttributes) == 1 {
		ref, refErr := expandSchemaElementReferenceQName(element, refAttributes[0], facts)
		if refErr != nil {
			return schemaElementParticleInput{}, refErr
		}
		return schemaElementParticleInput{
			loc:         element.loc,
			reference:   &schemaElementReferenceInput{name: ref, loc: refAttributes[0].loc},
			occurrences: occurrences,
		}, nil
	}
	if len(refAttributes) > 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "local element ref attribute is not unique")
	}
	block, _, err := schemaDeclarationBlockPolicy(
		element,
		facts.blockDefault,
		schemaBlockElementMask,
		version,
		schemaBlockElement,
	)
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	nameAttributes := syntaxAttributesByLocal(element, "name")
	if len(nameAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "local element input has an invalid name attribute")
	}
	local := collapseXMLWhitespace(nameAttributes[0].value)
	name, err := schemaLocalElementParticleName(element, local, facts, version, allowNamespacePolicy)
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	nillable, _, _, err := schemaElementBooleanAttribute(element, "nillable")
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	input := schemaElementParticleInput{
		loc:         element.loc,
		name:        name,
		occurrences: occurrences,
		nillable:    nillable,
		block:       block,
	}
	typeAttributes := syntaxAttributesByLocal(element, "type")
	if len(typeAttributes) == 0 {
		return input, nil
	}
	if len(typeAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "local element type attribute is not unique")
	}
	declaredType, err := expandSchemaQName(element, typeAttributes[0])
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	input.typeInput = &schemaElementInput{
		declaredType: declaredType,
		typeLoc:      typeAttributes[0].loc,
	}
	return input, nil
}

func expandSchemaElementReferenceQName(element *syntaxElement, attribute syntaxAttribute, facts schemaDocumentFacts) (QName, error) {
	ref, err := expandSchemaQName(element, attribute)
	if err != nil {
		return QName{}, err
	}
	if !facts.chameleon || !facts.targetNamespace.present || ref.Namespace() != "" {
		return ref, nil
	}
	qualified, err := NewQName(facts.targetNamespace.value, ref.Local())
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(attribute.loc, "construct chameleon element reference QName")
	}
	return qualified, nil
}

func schemaSimpleTypeInputFromElement(element *syntaxElement) (*schemaSimpleTypeInput, error) {
	if element == nil {
		return nil, newSchemaBridgeInvariant(Loc{}, "construct simple type input from a nil element")
	}
	var modelElement *syntaxElement
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI {
			continue
		}
		switch child.name.local {
		case "restriction", "list", "union":
			if modelElement != nil {
				return nil, newSchemaCompositionDiagnostic(child.loc, "simpleType requires exactly one model child")
			}
			modelElement = child
		}
	}
	if modelElement == nil {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simpleType requires exactly one model child")
	}
	var model schemaSimpleTypeModelInput
	var err error
	switch modelElement.name.local {
	case "restriction":
		model, err = schemaRestrictionModelInput(modelElement)
	case "list":
		model, err = schemaListModelInput(modelElement)
	case "union":
		model, err = schemaUnionModelInput(modelElement)
	default:
		return nil, newSchemaBridgeInvariant(modelElement.loc, "simple type has an unknown model child")
	}
	if err != nil {
		return nil, err
	}
	input := &schemaSimpleTypeInput{loc: element.loc, model: model}
	if restriction, ok := model.(*schemaSimpleTypeRestrictionModelInput); ok && restriction != nil && restriction.base.kind == schemaSimpleTypeQNameReferenceInput {
		input.base = restriction.base.name
		input.baseLoc = restriction.base.loc
		input.facets = cloneSchemaFacetInputs(restriction.facets)
	}
	return input, nil
}

func schemaLocalElementParticleName(element *syntaxElement, local string, facts schemaDocumentFacts, version XSDVersion, allowNamespacePolicy bool) (QName, error) {
	namespace, err := schemaLocalElementParticleNamespace(element, facts, version, allowNamespacePolicy)
	if err != nil {
		return QName{}, err
	}
	name, err := NewQName(namespace, local)
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(element.loc, "construct local element name")
	}
	return name, nil
}

func schemaLocalElementParticleNamespace(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion, allowNamespacePolicy bool) (string, error) {
	if !allowNamespacePolicy {
		return "", nil
	}
	targetNamespaceAttributes := syntaxAttributesByLocal(element, "targetNamespace")
	if len(targetNamespaceAttributes) > 0 {
		return schemaLocalElementTargetNamespace(targetNamespaceAttributes[0], facts, version)
	}
	if schemaLocalElementIsQualified(element, facts) {
		return facts.targetNamespace.value, nil
	}
	return "", nil
}

func schemaLocalElementTargetNamespace(attribute syntaxAttribute, facts schemaDocumentFacts, version XSDVersion) (string, error) {
	targetNamespace := collapseXMLWhitespace(attribute.value)
	if version == XSDVersion11 && (!facts.targetNamespace.present || targetNamespace != facts.targetNamespace.value) {
		return "", invalidSchemaLocalElementTargetNamespace(attribute, facts.targetNamespace)
	}
	return targetNamespace, nil
}

func schemaLocalElementIsQualified(element *syntaxElement, facts schemaDocumentFacts) bool {
	formAttributes := syntaxAttributesByLocal(element, "form")
	if len(formAttributes) == 1 {
		return collapseXMLWhitespace(formAttributes[0].value) == "qualified"
	}
	return facts.elementFormDefaultQualified
}

func invalidSchemaLocalElementTargetNamespace(attribute syntaxAttribute, targetNamespace schemaTargetNamespace) Diagnostic {
	message := "local element targetNamespace requires a containing schema targetNamespace"
	if targetNamespace.present {
		message = "local element targetNamespace must match the containing schema targetNamespace"
	}
	related := make([]Loc, 0, 1)
	if targetNamespace.loc != (Loc{}) {
		related = append(related, targetNamespace.loc)
	}
	return Diagnostic{
		class:   FailureInvalid,
		code:    invalidSchemaCompositionCode,
		loc:     attribute.loc,
		message: message,
		related: related,
		specRef: schemaElementTargetNamespaceXSD11SpecRef,
		cause:   errSchemaElementTargetNamespace,
	}
}

//nolint:gocognit // Keep restriction source cardinality and facet collection together.
func schemaRestrictionModelInput(element *syntaxElement) (schemaSimpleTypeModelInput, error) {
	baseAttributes := syntaxAttributesByLocal(element, "base")
	inline := inlineSimpleTypeChild(element)
	if len(baseAttributes) > 1 {
		return nil, newSchemaCompositionDiagnostic(baseAttributes[1].loc, "simple type restriction base attribute must be unique")
	}
	if len(baseAttributes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "simple type restriction cannot combine base with an inline simpleType")
	}
	if len(baseAttributes) == 0 && inline == nil {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simple type restriction requires a base attribute or inline simpleType")
	}
	var base schemaSimpleTypeReferenceInput
	if len(baseAttributes) == 1 {
		name, err := expandSchemaQName(element, baseAttributes[0])
		if err != nil {
			return nil, err
		}
		base = schemaSimpleTypeReferenceInput{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: name,
			loc:  baseAttributes[0].loc,
		}
	}
	if inline != nil {
		anonymous, err := schemaSimpleTypeInputFromElement(inline)
		if err != nil {
			return nil, err
		}
		base = schemaSimpleTypeReferenceInput{
			kind:      schemaSimpleTypeAnonymousReferenceInput,
			loc:       inline.loc,
			anonymous: anonymous,
		}
	}
	facets := make([]schemaFacetInput, 0)
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if _, ok := schemaFacetKindFromName(child.name.local); !ok {
			continue
		}
		facet, err := schemaFacetInputFromElement(child)
		if err != nil {
			return nil, err
		}
		facets = append(facets, *facet)
	}
	return &schemaSimpleTypeRestrictionModelInput{
		loc:    element.loc,
		base:   base,
		facets: facets,
	}, nil
}

func schemaListModelInput(element *syntaxElement) (schemaSimpleTypeModelInput, error) {
	itemTypes := syntaxAttributesByLocal(element, "itemType")
	inline := inlineSimpleTypeChild(element)
	if len(itemTypes) > 1 {
		return nil, newSchemaCompositionDiagnostic(itemTypes[1].loc, "simple type list itemType attribute must be unique")
	}
	if len(itemTypes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "simple type list cannot combine itemType with an inline simpleType")
	}
	if len(itemTypes) == 0 && inline == nil {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simple type list requires an itemType or inline simpleType")
	}
	var itemType schemaSimpleTypeReferenceInput
	if len(itemTypes) == 1 {
		name, err := expandSchemaQName(element, itemTypes[0])
		if err != nil {
			return nil, err
		}
		itemType = schemaSimpleTypeReferenceInput{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: name,
			loc:  itemTypes[0].loc,
		}
	}
	if inline != nil {
		anonymous, err := schemaSimpleTypeInputFromElement(inline)
		if err != nil {
			return nil, err
		}
		itemType = schemaSimpleTypeReferenceInput{
			kind:      schemaSimpleTypeAnonymousReferenceInput,
			loc:       inline.loc,
			anonymous: anonymous,
		}
	}
	return &schemaSimpleTypeListModelInput{
		loc:      element.loc,
		itemType: itemType,
	}, nil
}

func schemaUnionModelInput(element *syntaxElement) (schemaSimpleTypeModelInput, error) {
	members := make([]schemaSimpleTypeReferenceInput, 0)
	memberTypes := syntaxAttributesByLocal(element, "memberTypes")
	if len(memberTypes) > 1 {
		return nil, newSchemaCompositionDiagnostic(memberTypes[1].loc, "simple type union memberTypes attribute must be unique")
	}
	if len(memberTypes) == 1 {
		for _, token := range strings.Fields(collapseXMLWhitespace(memberTypes[0].value)) {
			attribute := memberTypes[0]
			attribute.value = token
			name, err := expandSchemaQName(element, attribute)
			if err != nil {
				return nil, err
			}
			members = append(members, schemaSimpleTypeReferenceInput{
				kind: schemaSimpleTypeQNameReferenceInput,
				name: name,
				loc:  memberTypes[0].loc,
			})
		}
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "simpleType" {
			continue
		}
		anonymous, err := schemaSimpleTypeInputFromElement(child)
		if err != nil {
			return nil, err
		}
		members = append(members, schemaSimpleTypeReferenceInput{
			kind:      schemaSimpleTypeAnonymousReferenceInput,
			loc:       child.loc,
			anonymous: anonymous,
		})
	}
	if len(members) == 0 {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simple type union requires memberTypes or an inline simpleType")
	}
	return &schemaSimpleTypeUnionModelInput{
		loc:     element.loc,
		members: members,
	}, nil
}

func schemaFacetInputFromElement(element *syntaxElement) (*schemaFacetInput, error) {
	kind, ok := schemaFacetKindFromName(element.name.local)
	if !ok {
		return nil, newSchemaBridgeInvariant(element.loc, fmt.Sprintf("schema facet <%s> has an unknown kind", element.name.local))
	}
	valueAttributes := syntaxAttributesByLocal(element, "value")
	if len(valueAttributes) != 1 {
		return nil, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			element.loc,
			element.name.local+" facet requires one value attribute",
			nil,
		)
	}
	fixedAttributes := syntaxAttributesByLocal(element, "fixed")
	if len(fixedAttributes) > 1 {
		return nil, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			element.loc,
			element.name.local+" facet fixed attribute must be unique",
			nil,
		)
	}
	fixed := false
	if len(fixedAttributes) == 1 {
		switch collapseXMLWhitespace(fixedAttributes[0].value) {
		case "true", "1":
			fixed = true
		case "false", "0":
		default:
			return nil, newSchemaCompositionDiagnostic(
				fixedAttributes[0].loc,
				fmt.Sprintf("attribute %q has an invalid boolean value", fixedAttributes[0].name.local),
			)
		}
	}
	return &schemaFacetInput{
		lexical:  valueAttributes[0].value,
		loc:      element.loc,
		valueLoc: valueAttributes[0].loc,
		kind:     kind,
		fixed:    fixed,
	}, nil
}

func schemaFacetKindFromName(name string) (schemaFacetKind, bool) {
	switch name {
	case "totalDigits":
		return schemaFacetTotalDigits, true
	case "fractionDigits":
		return schemaFacetFractionDigits, true
	case "minScale":
		return schemaFacetMinScale, true
	case "maxScale":
		return schemaFacetMaxScale, true
	case "pattern":
		return schemaFacetPattern, true
	case "enumeration":
		return schemaFacetEnumeration, true
	case "minInclusive":
		return schemaFacetMinInclusive, true
	case "minExclusive":
		return schemaFacetMinExclusive, true
	case "maxInclusive":
		return schemaFacetMaxInclusive, true
	case "maxExclusive":
		return schemaFacetMaxExclusive, true
	case "whiteSpace":
		return schemaFacetWhiteSpace, true
	case "length":
		return schemaFacetLength, true
	case "minLength":
		return schemaFacetMinLength, true
	case "maxLength":
		return schemaFacetMaxLength, true
	case "precision":
		return schemaFacetPrecision, true
	case "explicitTimezone":
		return schemaFacetExplicitTimezone, true
	default:
		return 0, false
	}
}

func schemaFacetName(kind schemaFacetKind) string {
	switch kind {
	case schemaFacetTotalDigits:
		return "totalDigits"
	case schemaFacetFractionDigits:
		return "fractionDigits"
	case schemaFacetMinScale:
		return "minScale"
	case schemaFacetMaxScale:
		return "maxScale"
	case schemaFacetPattern:
		return "pattern"
	case schemaFacetEnumeration:
		return "enumeration"
	case schemaFacetMinInclusive:
		return "minInclusive"
	case schemaFacetMinExclusive:
		return "minExclusive"
	case schemaFacetMaxInclusive:
		return "maxInclusive"
	case schemaFacetMaxExclusive:
		return "maxExclusive"
	case schemaFacetWhiteSpace:
		return "whiteSpace"
	case schemaFacetLength:
		return "length"
	case schemaFacetMinLength:
		return "minLength"
	case schemaFacetMaxLength:
		return "maxLength"
	case schemaFacetPrecision:
		return "precision"
	case schemaFacetExplicitTimezone:
		return "explicitTimezone"
	default:
		return ""
	}
}

func expandSchemaQName(element *syntaxElement, attribute syntaxAttribute) (QName, error) {
	lexeme := collapseXMLWhitespace(attribute.value)
	prefix, local, ok := splitConditionalQName(lexeme)
	if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
		return QName{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaConditionalCode,
			attribute.loc,
			fmt.Sprintf("attribute %q has a malformed QName", attribute.name.local),
			nil,
		)
	}
	namespace := ""
	if prefix == "" {
		namespace, _ = element.scope.lookup("")
	}
	if prefix != "" {
		var bound bool
		namespace, bound = element.scope.lookup(prefix)
		if !bound {
			return QName{}, newDiagnostic(
				FailureInvalid,
				invalidSchemaConditionalCode,
				attribute.loc,
				fmt.Sprintf("attribute %q has an unbound QName prefix", attribute.name.local),
				nil,
			)
		}
	}
	name, err := NewQName(namespace, local)
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(attribute.loc, "construct expanded schema QName")
	}
	return name, nil
}

func expandSchemaSubstitutionGroupQName(element *syntaxElement, attribute syntaxAttribute, item string, version XSDVersion) (QName, error) {
	if err := validateSchemaElementSubstitutionGroupQName(element, attribute, item, version); err != nil {
		return QName{}, err
	}
	prefix, local, ok := splitConditionalQName(item)
	if !ok {
		return QName{}, newSchemaBridgeInvariant(attribute.loc, "validated substitutionGroup QName could not be split")
	}
	namespace := ""
	if prefix == "" {
		namespace, _ = element.scope.lookup("")
	}
	if prefix != "" {
		var bound bool
		namespace, bound = element.scope.lookup(prefix)
		if !bound {
			return QName{}, newSchemaBridgeInvariant(attribute.loc, "validated substitutionGroup QName prefix is unbound")
		}
	}
	name, err := NewQName(namespace, local)
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(attribute.loc, "construct expanded substitutionGroup QName")
	}
	return name, nil
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
	value := collapseXMLWhitespace(attributes[0].value)
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
		if attribute.name.namespace == "" && attribute.name.local == local {
			attributes = append(attributes, attribute)
		}
	}
	return attributes
}

func schemaNotationInputFromElement(element *syntaxElement, version XSDVersion) (*schemaNotationInput, error) {
	publicAttributes := syntaxAttributesByLocal(element, "public")
	if len(publicAttributes) == 0 {
		return nil, newSchemaNotationDiagnostic(
			element.loc,
			"notation declaration requires a public attribute",
			version,
			errSchemaNotationPublic,
		)
	}
	if len(publicAttributes) != 1 {
		return nil, newSchemaCompositionDiagnostic(publicAttributes[1].loc, "notation public attribute must be unique")
	}
	public := publicAttributes[0]
	if err := validateSchemaNotationAttribute(public, version); err != nil {
		return nil, err
	}
	notation := &schemaNotationInput{
		public:    collapseXMLWhitespace(public.value),
		publicLoc: public.loc,
	}
	systemAttributes := syntaxAttributesByLocal(element, "system")
	if len(systemAttributes) == 0 {
		return notation, nil
	}
	if len(systemAttributes) != 1 {
		return nil, newSchemaCompositionDiagnostic(systemAttributes[1].loc, "notation system attribute must be unique")
	}
	system := systemAttributes[0]
	if err := validateSchemaNotationAttribute(system, version); err != nil {
		return nil, err
	}
	notation.system = collapseXMLWhitespace(system.value)
	notation.systemLoc = system.loc
	notation.hasSystem = true
	return notation, nil
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

type schemaSimpleTypeResult struct {
	present          bool
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

type schemaSimpleTypeAtomicKind uint8

const (
	schemaSimpleTypeAtomicUnknown schemaSimpleTypeAtomicKind = iota
	schemaSimpleTypeAtomicString
	schemaSimpleTypeAtomicToken
	schemaSimpleTypeAtomicInteger
	schemaSimpleTypeAtomicNegativeInteger
	schemaSimpleTypeAtomicDecimal
	schemaSimpleTypeAtomicPrecisionDecimal
	schemaSimpleTypeAtomicLanguage
	schemaSimpleTypeAtomicNCName
	schemaSimpleTypeAtomicAnyURI
	schemaSimpleTypeAtomicID
)

func schemaSimpleTypeAtomicKindIsUnsupported(kind schemaSimpleTypeAtomicKind) bool {
	switch kind {
	case schemaSimpleTypeAtomicLanguage,
		schemaSimpleTypeAtomicNCName,
		schemaSimpleTypeAtomicAnyURI,
		schemaSimpleTypeAtomicID:
		return true
	case schemaSimpleTypeAtomicUnknown,
		schemaSimpleTypeAtomicString,
		schemaSimpleTypeAtomicToken,
		schemaSimpleTypeAtomicInteger,
		schemaSimpleTypeAtomicNegativeInteger,
		schemaSimpleTypeAtomicDecimal,
		schemaSimpleTypeAtomicPrecisionDecimal:
		return false
	default:
		return false
	}
}

type schemaSimpleTypeState uint8

const (
	schemaSimpleTypeUnvisited schemaSimpleTypeState = iota
	schemaSimpleTypeVisiting
	schemaSimpleTypeResolved
)

type schemaSimpleTypeResolver struct {
	records          []schemaComponentRecord
	byName           map[QName][]int
	visibleSources   map[SourceID][]SourceID
	states           map[*schemaSimpleTypeInput]schemaSimpleTypeState
	inputResults     map[*schemaSimpleTypeInput]schemaSimpleTypeResult
	results          []schemaSimpleTypeResult
	stack            []*schemaSimpleTypeInput
	stackFallbackLoc []Loc
}

type schemaSimpleTypeResolution struct {
	results  []schemaSimpleTypeResult
	byInput  map[*schemaSimpleTypeInput]schemaSimpleTypeResult
	resolver *schemaSimpleTypeResolver
}

func resolveSchemaSimpleTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	version XSDVersion,
) (schemaSimpleTypeResolution, error) {
	resolver := schemaSimpleTypeResolver{
		records:          records,
		byName:           byName,
		visibleSources:   visibleSources,
		states:           make(map[*schemaSimpleTypeInput]schemaSimpleTypeState),
		inputResults:     make(map[*schemaSimpleTypeInput]schemaSimpleTypeResult),
		results:          make([]schemaSimpleTypeResult, len(records)),
		stack:            make([]*schemaSimpleTypeInput, 0),
		stackFallbackLoc: make([]Loc, 0),
	}
	for index, record := range records {
		if record.simpleType == nil {
			continue
		}
		if _, err := resolver.resolve(index, version); err != nil {
			return schemaSimpleTypeResolution{}, err
		}
	}
	for _, record := range records {
		if record.element == nil || record.element.inlineSimpleType == nil {
			continue
		}
		if _, err := resolver.resolveInput(record.element.inlineSimpleType, record.element.inlineSimpleType.loc, true, record.id.Source(), version); err != nil {
			return schemaSimpleTypeResolution{}, err
		}
	}
	return schemaSimpleTypeResolution{
		results:  resolver.results,
		byInput:  resolver.inputResults,
		resolver: &resolver,
	}, nil
}

type schemaAttributeTypeResult struct {
	present          bool
	typeReference    schemaSimpleTypeReferenceComponent
	hasTypeReference bool
}

func resolvedSchemaAttributeTypeResult(reference schemaSimpleTypeReferenceComponent) schemaAttributeTypeResult {
	return schemaAttributeTypeResult{
		present:          true,
		typeReference:    reference,
		hasTypeReference: true,
	}
}

func resolveSchemaAttributeTypes(
	records []schemaComponentRecord,
	simpleTypes schemaSimpleTypeResolution,
	version XSDVersion,
) ([]schemaAttributeTypeResult, error) {
	if len(simpleTypes.results) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "attribute type resolution has incomplete simple type results")
	}
	if simpleTypes.resolver == nil {
		return nil, newSchemaBridgeInvariant(Loc{}, "attribute type resolution has no simple type resolver")
	}
	results := make([]schemaAttributeTypeResult, len(records))
	for index, record := range records {
		if record.attribute == nil {
			continue
		}
		result, err := resolveSchemaAttributeType(record, simpleTypes.resolver, version)
		if err != nil {
			return nil, err
		}
		results[index] = result
	}
	return results, nil
}

func resolveSchemaAttributeType(
	record schemaComponentRecord,
	resolver *schemaSimpleTypeResolver,
	version XSDVersion,
) (schemaAttributeTypeResult, error) {
	input := record.attribute
	if input == nil {
		return schemaAttributeTypeResult{}, newSchemaBridgeInvariant(record.loc, "attribute type resolution has no type input")
	}
	if resolver == nil {
		return schemaAttributeTypeResult{}, newSchemaBridgeInvariant(input.typeLoc, "attribute type resolution has no simple type resolver")
	}
	reference, err := resolver.resolveReference(schemaSimpleTypeReferenceInput{
		kind: schemaSimpleTypeQNameReferenceInput,
		name: input.declaredType,
		loc:  input.typeLoc,
	}, record.id.Source(), version)
	if err != nil {
		return schemaAttributeTypeResult{}, reframeSchemaAttributeReferenceError(input, err, version)
	}
	if !schemaAttributeTypeReferenceSupported(reference) {
		return schemaAttributeTypeResult{}, unsupportedSchemaAttributeType(
			input,
			version,
			fmt.Sprintf("attribute type %q has an unsupported simple type model", input.declaredType),
		)
	}
	return resolvedSchemaAttributeTypeResult(reference), nil
}

func schemaAttributeTypeReferenceSupported(reference schemaSimpleTypeReferenceComponent) bool {
	if reference.variety != SimpleTypeVarietyAtomicRestriction {
		return false
	}
	switch reference.atomicKind {
	case schemaSimpleTypeAtomicInteger, schemaSimpleTypeAtomicDecimal,
		schemaSimpleTypeAtomicToken, schemaSimpleTypeAtomicLanguage, schemaSimpleTypeAtomicNCName,
		schemaSimpleTypeAtomicAnyURI, schemaSimpleTypeAtomicID:
		return true
	case schemaSimpleTypeAtomicUnknown,
		schemaSimpleTypeAtomicString,
		schemaSimpleTypeAtomicNegativeInteger,
		schemaSimpleTypeAtomicPrecisionDecimal:
		return false
	default:
		return false
	}
}

func reframeSchemaAttributeReferenceError(input *schemaAttributeInput, err error, version XSDVersion) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return err
	}
	if errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
		attributeErr := unresolvedSchemaAttributeTypeWithCause(
			input,
			version,
			errors.Join(errSchemaAttributeTypeUnresolved, err),
		)
		return attributeErr
	}
	if errors.Is(err, errSchemaSimpleTypeBaseWrongKind) {
		attributeErr := wrongKindSchemaAttributeTypeWithCause(
			input,
			diagnostic.Related(),
			version,
			errors.Join(errSchemaAttributeTypeWrongKind, err),
		)
		return attributeErr
	}
	if errors.Is(err, errSchemaSimpleTypeBaseAmbiguous) {
		attributeErr := ambiguousSchemaAttributeTypeWithCause(
			input,
			diagnostic.Related(),
			version,
			errors.Join(errSchemaAttributeTypeAmbiguous, err),
		)
		return attributeErr
	}
	return err
}

func unresolvedSchemaAttributeTypeWithCause(input *schemaAttributeInput, version XSDVersion, cause error) error {
	return newSchemaAttributeTypeDiagnostic(
		diagnosticSchemaAttributeTypeUnresolvedCode,
		input.typeLoc,
		fmt.Sprintf("attribute type %q cannot be resolved", input.declaredType),
		nil,
		version,
		cause,
	)
}

func wrongKindSchemaAttributeTypeWithCause(input *schemaAttributeInput, related []Loc, version XSDVersion, cause error) error {
	return newSchemaAttributeTypeDiagnostic(
		diagnosticSchemaAttributeTypeWrongKindCode,
		input.typeLoc,
		fmt.Sprintf("attribute type %q does not name a simple type", input.declaredType),
		related,
		version,
		cause,
	)
}

func ambiguousSchemaAttributeTypeWithCause(input *schemaAttributeInput, related []Loc, version XSDVersion, cause error) error {
	return newSchemaAttributeTypeDiagnostic(
		diagnosticSchemaAttributeTypeAmbiguousCode,
		input.typeLoc,
		fmt.Sprintf("attribute type %q is ambiguous", input.declaredType),
		related,
		version,
		cause,
	)
}

func unsupportedSchemaAttributeType(input *schemaAttributeInput, version XSDVersion, message string) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			input.typeLoc,
			"schema syntax feature is not registered",
			fmt.Errorf("%w: %q", errSchemaAttributeTypeUnsupported, input.declaredType),
		)
	}
	diagnostic := newUnsupportedForVersionWithCause(
		feature,
		UnsupportedSchemaSyntaxCode,
		input.typeLoc,
		message,
		version,
		fmt.Errorf("%w: %q", errSchemaAttributeTypeUnsupported, input.declaredType),
	)
	if diagnostic.Class() == FailureUnsupported {
		diagnostic.specRef = schemaAttributeTypeSpecRef(version)
	}
	return diagnostic
}

func newSchemaAttributeTypeDiagnostic(code string, loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaAttributeTypeSpecRef(version),
		cause:   cause,
	}
}

func schemaAttributeTypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaAttributeTypeXSD10SpecRef
	}
	return schemaAttributeTypeXSD11SpecRef
}

// reframeSchemaAttributeTypeCycle retains the shared simple-type cycle cause
// while making an attribute's type expression the useful primary context.
func reframeSchemaAttributeTypeCycle(records []schemaComponentRecord, byName map[QName][]int, err error, version XSDVersion) error {
	if !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
		return nil
	}
	var cycleDiagnostic Diagnostic
	if !errors.As(err, &cycleDiagnostic) {
		return nil
	}
	for _, record := range records {
		input := record.attribute
		if input == nil || input.declaredType.Namespace() == xsdNamespaceURI {
			continue
		}
		candidates := schemaSimpleTypeRecordIndices(input.declaredType, records, byName)
		if len(candidates) != 1 {
			continue
		}
		graphLocations := make([]Loc, 0, 4)
		collectSchemaSimpleTypeGraphLocations(
			candidates[0],
			records,
			byName,
			input.typeLoc,
			&graphLocations,
			make([]uint8, len(records)),
		)
		if !schemaLocationListContains(graphLocations, cycleDiagnostic.Loc()) {
			continue
		}
		related := make([]Loc, 0, len(cycleDiagnostic.Related())+4)
		related = appendSchemaRelatedLocation(related, record.loc, input.typeLoc)
		related = appendSchemaRelatedLocation(related, cycleDiagnostic.Loc(), input.typeLoc)
		for _, relatedLoc := range cycleDiagnostic.Related() {
			related = appendSchemaRelatedLocation(related, relatedLoc, input.typeLoc)
		}
		for _, graphLoc := range graphLocations {
			related = appendSchemaRelatedLocation(related, graphLoc, input.typeLoc)
		}
		return newSchemaAttributeTypeDiagnostic(
			diagnosticSchemaAttributeTypeCycleCode,
			input.typeLoc,
			fmt.Sprintf("attribute type %q resolves through cyclic simple type definitions", input.declaredType),
			related,
			version,
			err,
		)
	}
	return nil
}

func schemaLocationListContains(locations []Loc, wanted Loc) bool {
	for _, loc := range locations {
		if loc == wanted {
			return true
		}
	}
	return false
}

func schemaSimpleTypeRecordIndices(name QName, records []schemaComponentRecord, byName map[QName][]int) []int {
	candidates := byName[name]
	indices := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if records[candidate].kind == ComponentKindSimpleTypeDefinition {
			indices = append(indices, candidate)
		}
	}
	return indices
}

func collectSchemaSimpleTypeGraphLocations(
	index int,
	records []schemaComponentRecord,
	byName map[QName][]int,
	primary Loc,
	related *[]Loc,
	states []uint8,
) {
	if index < 0 || index >= len(records) || records[index].simpleType == nil || states[index] != 0 {
		return
	}
	states[index] = 1
	*related = appendSchemaRelatedLocation(*related, records[index].loc, primary)
	input := records[index].simpleType
	for _, reference := range schemaSimpleTypeReferences(input) {
		*related = appendSchemaRelatedLocation(*related, reference.loc, primary)
		if reference.kind != schemaSimpleTypeQNameReferenceInput || reference.name.Namespace() == xsdNamespaceURI {
			continue
		}
		for _, dependency := range schemaSimpleTypeRecordIndices(reference.name, records, byName) {
			collectSchemaSimpleTypeGraphLocations(dependency, records, byName, primary, related, states)
		}
	}
	states[index] = 2
}

func schemaSimpleTypeReferences(input *schemaSimpleTypeInput) []schemaSimpleTypeReferenceInput {
	if input == nil {
		return nil
	}
	model := input.model
	if model == nil && !input.base.IsZero() {
		return []schemaSimpleTypeReferenceInput{{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: input.base,
			loc:  input.baseLoc,
		}}
	}
	if model == nil {
		return nil
	}
	switch typed := model.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if typed == nil {
			return nil
		}
		return []schemaSimpleTypeReferenceInput{typed.base}
	case *schemaSimpleTypeListModelInput:
		if typed == nil {
			return nil
		}
		return []schemaSimpleTypeReferenceInput{typed.itemType}
	case *schemaSimpleTypeUnionModelInput:
		if typed == nil {
			return nil
		}
		return append([]schemaSimpleTypeReferenceInput(nil), typed.members...)
	default:
		return nil
	}
}

func appendSchemaRelatedLocation(related []Loc, candidate, primary Loc) []Loc {
	if candidate.IsZero() || candidate == primary {
		return related
	}
	for _, existing := range related {
		if existing == candidate {
			return related
		}
	}
	return append(related, candidate)
}

type schemaElementTypeResult struct {
	present           bool
	declaredType      QName
	typeID            ComponentID
	hasTypeID         bool
	typeReference     schemaSimpleTypeReferenceComponent
	hasTypeReference  bool
	abstract          bool
	nillable          bool
	block             schemaBlockPolicy
	substitutionGroup []schemaElementSubstitutionGroup
}

func resolvedSchemaElementTypeResult(input *schemaElementInput, typeID ComponentID, hasTypeID bool) schemaElementTypeResult {
	return schemaElementTypeResult{
		present:      true,
		declaredType: input.declaredType,
		typeID:       typeID,
		hasTypeID:    hasTypeID,
		abstract:     input.abstract,
		nillable:     input.nillable,
		block:        input.block,
	}
}

type schemaScalarTypeScope uint8

const (
	schemaScalarTypeGlobalElement schemaScalarTypeScope = iota + 1
	schemaScalarTypeLocalParticle
)

func (scope schemaScalarTypeScope) allowsBoolean() bool {
	return scope == schemaScalarTypeGlobalElement || scope == schemaScalarTypeLocalParticle
}

func resolveSchemaElementTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes schemaSimpleTypeResolution,
	complexTypes []schemaComplexTypeResult,
	version XSDVersion,
) ([]schemaElementTypeResult, error) {
	if len(simpleTypes.results) != len(records) || len(complexTypes) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "element type resolution has incomplete type results")
	}
	results := make([]schemaElementTypeResult, len(records))
	for index, record := range records {
		if record.element == nil {
			continue
		}
		result, err := resolveSchemaElementType(record, records, byName, visibleSources, simpleTypes, complexTypes, version)
		if err != nil {
			return nil, err
		}
		results[index] = result
	}
	return results, nil
}

type schemaSubstitutionTypeClass uint8

const (
	schemaSubstitutionTypeUnknown schemaSubstitutionTypeClass = iota
	schemaSubstitutionTypeBoolean
	schemaSubstitutionTypeInteger
	schemaSubstitutionTypeDecimal
	schemaSubstitutionTypePrecisionDecimal
)

type schemaSubstitutionTypeRelation uint8

const (
	schemaSubstitutionTypeRelationUnknown schemaSubstitutionTypeRelation = iota
	schemaSubstitutionTypeRelationValid
	schemaSubstitutionTypeRelationInvalid
)

func resolveSchemaElementSubstitutionGroups(
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes schemaSimpleTypeResolution,
	elements []schemaElementTypeResult,
	edges []syntaxDocumentEdge,
	sourceNamespaces map[SourceID]string,
	version XSDVersion,
) ([]schemaElementTypeResult, error) {
	if len(elements) != len(records) || len(simpleTypes.results) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "substitution-group resolution has incomplete component results")
	}
	results := make([]schemaElementTypeResult, len(elements))
	copy(results, elements)
	for index := range results {
		results[index].substitutionGroup = cloneSchemaElementSubstitutionGroups(elements[index].substitutionGroup)
	}
	for index, record := range records {
		input := record.element
		if input == nil || len(input.substitutionGroup) == 0 {
			continue
		}
		if !results[index].present {
			return nil, newSchemaSubstitutionUnsupported(
				input.substitutionGroup[0].loc,
				"substitutionGroup affiliations for this global element shape are not implemented",
				version,
				errSchemaSubstitutionTypeUnsupported,
			)
		}
		sourceClass, supported := schemaSubstitutionElementTypeClass(record, results[index])
		if !supported {
			return nil, newSchemaSubstitutionUnsupported(
				input.substitutionGroup[0].loc,
				"substitutionGroup affiliations for this global element type are not implemented",
				version,
				errSchemaSubstitutionTypeUnsupported,
			)
		}
		affiliations, err := resolveSchemaElementSubstitutionGroupForInputs(
			index,
			record,
			records,
			byName,
			input.substitutionGroup,
			edges,
			sourceNamespaces,
			visibleSources,
			sourceClass,
			results,
			version,
		)
		if err != nil {
			return nil, err
		}
		results[index].substitutionGroup = affiliations
	}
	if err := rejectSchemaSubstitutionCycles(records, results, version); err != nil {
		return nil, err
	}
	return results, nil
}

func schemaSubstitutionElementTypeClass(record schemaComponentRecord, result schemaElementTypeResult) (schemaSubstitutionTypeClass, bool) {
	if record.element == nil || record.element.inlineSimpleType != nil || !result.present || !result.hasTypeReference {
		return schemaSubstitutionTypeUnknown, false
	}
	return schemaSubstitutionTypeClassFromReference(result.typeReference)
}

func schemaSubstitutionTypeClassFromReference(reference schemaSimpleTypeReferenceComponent) (schemaSubstitutionTypeClass, bool) {
	if schemaSimpleTypeAtomicKindIsUnsupported(reference.atomicKind) {
		return schemaSubstitutionTypeUnknown, false
	}
	switch facets := reference.facets.(type) {
	case schemaBooleanFacetVariant:
		return schemaSubstitutionTypeBoolean, true
	case schemaIntegerFacetVariant:
		return schemaSubstitutionTypeInteger, true
	case schemaDecimalFacetVariant:
		return schemaSubstitutionTypeDecimal, true
	case schemaPrecisionDecimalFacetVariant:
		return schemaSubstitutionTypePrecisionDecimal, true
	case schemaDigitFacetVariant:
		switch facets.value.Kind() {
		case DigitDatatypeInteger:
			return schemaSubstitutionTypeInteger, true
		case DigitDatatypeDecimal:
			return schemaSubstitutionTypeDecimal, true
		default:
			return schemaSubstitutionTypeUnknown, false
		}
	default:
		return schemaSubstitutionTypeUnknown, false
	}
}

// resolveSchemaElementSubstitutionGroup retains the phase-local helper used by
// construction tests. The public discovery path calls the richer input-aware
// variant below after element type resolution has completed.
func resolveSchemaElementSubstitutionGroup(
	sourceIndex int,
	source schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	edges []syntaxDocumentEdge,
	sourceNamespaces map[SourceID]string,
	version XSDVersion,
) ([]schemaElementSubstitutionGroup, error) {
	if source.element == nil {
		return nil, newSchemaBridgeInvariant(source.loc, "substitution-group source has no element input")
	}
	allSources := make([]SourceID, 0, len(records))
	for _, record := range records {
		candidateSource := record.id.Source()
		if sourceIDInList(allSources, candidateSource) {
			continue
		}
		allSources = append(allSources, candidateSource)
	}
	return resolveSchemaElementSubstitutionGroupForInputs(
		sourceIndex,
		source,
		records,
		byName,
		source.element.substitutionGroup,
		edges,
		sourceNamespaces,
		map[SourceID][]SourceID{source.id.Source(): allSources},
		schemaSubstitutionTypeUnknown,
		make([]schemaElementTypeResult, len(records)),
		version,
	)
}

func resolveSchemaElementSubstitutionGroupForInputs(
	sourceIndex int,
	source schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	inputs []schemaElementSubstitutionGroupInput,
	edges []syntaxDocumentEdge,
	sourceNamespaces map[SourceID]string,
	visibleSources map[SourceID][]SourceID,
	sourceClass schemaSubstitutionTypeClass,
	elements []schemaElementTypeResult,
	version XSDVersion,
) ([]schemaElementSubstitutionGroup, error) {
	sourceNamespace, ok := sourceNamespaces[source.id.Source()]
	if !ok {
		return nil, newSchemaBridgeInvariant(source.loc, "substitution-group source has no target namespace")
	}
	seen := make(map[QName]struct{}, len(inputs))
	affiliations := make([]schemaElementSubstitutionGroup, 0, len(inputs))
	for _, input := range inputs {
		if _, duplicate := seen[input.name]; duplicate {
			continue
		}
		seen[input.name] = struct{}{}
		affiliation, err := resolveSchemaElementSubstitutionGroupInput(
			sourceIndex,
			source,
			input,
			records,
			byName,
			edges,
			sourceNamespace,
			visibleSources,
			sourceClass,
			elements,
			version,
		)
		if err != nil {
			return nil, err
		}
		affiliations = append(affiliations, affiliation)
	}
	return affiliations, nil
}

func schemaSubstitutionGroupHeadIndex(
	input schemaElementSubstitutionGroupInput,
	source schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	sourceNamespace string,
	version XSDVersion,
) (int, error) {
	candidates := byName[input.name]
	if len(candidates) == 0 {
		return 0, newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionUnresolvedCode,
			input.loc,
			fmt.Sprintf("substitutionGroup head %q cannot be resolved", input.name),
			nil,
			errSchemaSubstitutionUnresolved,
			schemaSubstitutionResolveSpecRef(version),
		)
	}
	elementCandidates := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate < 0 || candidate >= len(records) {
			return 0, newSchemaBridgeInvariant(input.loc, "substitution-group head lookup has an invalid record index")
		}
		if records[candidate].kind != ComponentKindElementDeclaration {
			continue
		}
		elementCandidates = append(elementCandidates, candidate)
	}
	if len(elementCandidates) == 0 {
		return 0, newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionWrongKindCode,
			input.loc,
			fmt.Sprintf("substitutionGroup head %q is not a global element declaration", input.name),
			schemaComponentLocations(records, candidates),
			errSchemaSubstitutionWrongKind,
			schemaSubstitutionResolveSpecRef(version),
		)
	}
	visibleCandidates, err := schemaElementReferenceVisibleCandidates(elementCandidates, source, records, visibleSources)
	if err != nil {
		return 0, err
	}
	if len(visibleCandidates) == 0 {
		return 0, schemaSubstitutionGroupVisibilityDiagnostic(input, source, sourceNamespace, records, elementCandidates, version)
	}
	if len(visibleCandidates) > 1 {
		return 0, newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionAmbiguousCode,
			input.loc,
			fmt.Sprintf("substitutionGroup head %q is ambiguous", input.name),
			schemaComponentLocations(records, visibleCandidates),
			errSchemaSubstitutionAmbiguous,
			schemaSubstitutionResolveSpecRef(version),
		)
	}
	return visibleCandidates[0], nil
}

func schemaSubstitutionGroupVisibilityDiagnostic(
	input schemaElementSubstitutionGroupInput,
	source schemaComponentRecord,
	sourceNamespace string,
	records []schemaComponentRecord,
	elementCandidates []int,
	version XSDVersion,
) error {
	related := schemaComponentLocations(records, elementCandidates)
	if input.name.Namespace() != sourceNamespace {
		return newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionImportCode,
			input.loc,
			fmt.Sprintf("substitutionGroup head %q is not visible from schema document %q", input.name, source.id.Source()),
			related,
			errSchemaSubstitutionImport,
			schemaSubstitutionConstraintSpecRef(version),
		)
	}
	return newSchemaSubstitutionGroupDiagnostic(
		diagnosticSchemaSubstitutionUnresolvedCode,
		input.loc,
		fmt.Sprintf("substitutionGroup head %q is not visible from schema document %q", input.name, source.id.Source()),
		related,
		errSchemaSubstitutionUnresolved,
		schemaSubstitutionResolveSpecRef(version),
	)
}

func resolveSchemaElementSubstitutionGroupInput(
	sourceIndex int,
	source schemaComponentRecord,
	input schemaElementSubstitutionGroupInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	edges []syntaxDocumentEdge,
	sourceNamespace string,
	visibleSources map[SourceID][]SourceID,
	sourceClass schemaSubstitutionTypeClass,
	elements []schemaElementTypeResult,
	version XSDVersion,
) (schemaElementSubstitutionGroup, error) {
	targetIndex, err := schemaSubstitutionGroupHeadIndex(input, source, records, byName, visibleSources, sourceNamespace, version)
	if err != nil {
		return schemaElementSubstitutionGroup{}, err
	}
	if targetIndex == sourceIndex {
		return schemaElementSubstitutionGroup{}, newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionSelfCode,
			input.loc,
			fmt.Sprintf("global element %q cannot affiliate with itself", source.name),
			[]Loc{source.loc},
			errSchemaSubstitutionSelf,
			schemaSubstitutionConstraintSpecRef(version),
		)
	}
	if input.name.Namespace() != sourceNamespace && !schemaSubstitutionNamespacePermitted(version, input.name.Namespace()) && !hasDirectSchemaImport(edges, source.id.Source(), input.name.Namespace()) {
		return schemaElementSubstitutionGroup{}, newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionImportCode,
			input.loc,
			fmt.Sprintf("substitutionGroup head %q is in a foreign namespace without a direct import", input.name),
			[]Loc{records[targetIndex].loc},
			errSchemaSubstitutionImport,
			schemaSubstitutionConstraintSpecRef(version),
		)
	}
	targetClass, supported := schemaSubstitutionElementTypeClass(records[targetIndex], elements[targetIndex])
	if !supported {
		return schemaElementSubstitutionGroup{}, newSchemaSubstitutionUnsupported(
			input.loc,
			fmt.Sprintf("substitutionGroup head %q has an unsupported or untyped global element shape", input.name),
			version,
			errSchemaSubstitutionTypeUnsupported,
		)
	}
	relation := schemaSubstitutionTypeRelationFor(source, records[targetIndex], sourceClass, targetClass)
	if relation == schemaSubstitutionTypeRelationInvalid {
		return schemaElementSubstitutionGroup{}, newSchemaSubstitutionGroupDiagnostic(
			diagnosticSchemaSubstitutionTypeCode,
			input.loc,
			fmt.Sprintf("substitutionGroup member %q is not derived from head %q", source.name, input.name),
			[]Loc{records[targetIndex].loc},
			errSchemaSubstitutionTypeInvalid,
			schemaSubstitutionConstraintSpecRef(version),
		)
	}
	if relation != schemaSubstitutionTypeRelationValid {
		return schemaElementSubstitutionGroup{}, newSchemaSubstitutionUnsupported(
			input.loc,
			fmt.Sprintf("substitutionGroup type derivation from %q to %q is not implemented", source.element.declaredType, records[targetIndex].element.declaredType),
			version,
			errSchemaSubstitutionTypeUnsupported,
		)
	}
	return schemaElementSubstitutionGroup{
		targetID: records[targetIndex].id,
		loc:      input.loc,
	}, nil
}

func schemaSubstitutionTypeRelationFor(
	source schemaComponentRecord,
	target schemaComponentRecord,
	sourceClass schemaSubstitutionTypeClass,
	targetClass schemaSubstitutionTypeClass,
) schemaSubstitutionTypeRelation {
	if source.element == nil || target.element == nil {
		return schemaSubstitutionTypeRelationUnknown
	}
	sourceType := source.element.declaredType
	targetType := target.element.declaredType
	if sourceType.IsZero() || targetType.IsZero() {
		return schemaSubstitutionTypeRelationUnknown
	}
	if sourceType == targetType {
		return schemaSubstitutionTypeRelationValid
	}
	if relation, handled := schemaSubstitutionBooleanRelation(sourceType, targetType, sourceClass, targetClass); handled {
		return relation
	}
	if relation, handled := schemaSubstitutionSameClassRelation(sourceType, targetType, sourceClass, targetClass); handled {
		return relation
	}
	if relation, handled := schemaSubstitutionNumericRelation(sourceType, targetType, sourceClass, targetClass); handled {
		return relation
	}
	return schemaSubstitutionTypeRelationUnknown
}

func schemaSubstitutionBooleanRelation(
	sourceType QName,
	targetType QName,
	sourceClass schemaSubstitutionTypeClass,
	targetClass schemaSubstitutionTypeClass,
) (schemaSubstitutionTypeRelation, bool) {
	sourceBoolean := sourceClass == schemaSubstitutionTypeBoolean
	targetBoolean := targetClass == schemaSubstitutionTypeBoolean
	if !sourceBoolean && !targetBoolean {
		return schemaSubstitutionTypeRelationUnknown, false
	}
	if sourceBoolean != targetBoolean {
		return schemaSubstitutionTypeRelationInvalid, true
	}
	if sourceType.Namespace() != xsdNamespaceURI && targetType.Namespace() == xsdNamespaceURI {
		return schemaSubstitutionTypeRelationValid, true
	}
	return schemaSubstitutionTypeRelationUnknown, true
}

func schemaSubstitutionSameClassRelation(
	sourceType QName,
	targetType QName,
	sourceClass schemaSubstitutionTypeClass,
	targetClass schemaSubstitutionTypeClass,
) (schemaSubstitutionTypeRelation, bool) {
	if sourceClass != targetClass {
		return schemaSubstitutionTypeRelationUnknown, false
	}
	if sourceType.Namespace() != xsdNamespaceURI && targetType.Namespace() == xsdNamespaceURI {
		return schemaSubstitutionTypeRelationValid, true
	}
	return schemaSubstitutionTypeRelationUnknown, true
}

func schemaSubstitutionNumericRelation(
	sourceType QName,
	targetType QName,
	sourceClass schemaSubstitutionTypeClass,
	targetClass schemaSubstitutionTypeClass,
) (schemaSubstitutionTypeRelation, bool) {
	if sourceClass == schemaSubstitutionTypeInteger && targetClass == schemaSubstitutionTypeDecimal {
		if targetType.Namespace() == xsdNamespaceURI && targetType.Local() == "decimal" {
			return schemaSubstitutionTypeRelationValid, true
		}
		return schemaSubstitutionTypeRelationUnknown, true
	}
	if sourceClass == schemaSubstitutionTypeDecimal && targetClass == schemaSubstitutionTypeInteger {
		if sourceType.Namespace() == xsdNamespaceURI && sourceType.Local() == "decimal" {
			return schemaSubstitutionTypeRelationInvalid, true
		}
		return schemaSubstitutionTypeRelationUnknown, true
	}
	return schemaSubstitutionTypeRelationUnknown, false
}

func schemaSubstitutionNamespacePermitted(version XSDVersion, namespace string) bool {
	if version != XSDVersion11 {
		return false
	}
	return namespace == xsdNamespaceURI || namespace == schemaInstanceNamespaceURI
}

func hasDirectSchemaImport(edges []syntaxDocumentEdge, source SourceID, namespace string) bool {
	for _, edge := range edges {
		if edge.source != source || edge.kind != syntaxReferenceImport {
			continue
		}
		if edge.hasNamespace {
			if edge.namespaceURN == namespace {
				return true
			}
			continue
		}
		if namespace == "" {
			return true
		}
	}
	return false
}

type schemaSubstitutionCycleChecker struct {
	elements  []schemaElementTypeResult
	indices   map[ComponentID]int
	state     []uint8
	pathNodes []int
	pathEdges []schemaElementSubstitutionGroup
	version   XSDVersion
}

func rejectSchemaSubstitutionCycles(records []schemaComponentRecord, elements []schemaElementTypeResult, version XSDVersion) error {
	if len(records) != len(elements) {
		return newSchemaBridgeInvariant(Loc{}, "substitution-group cycle check has incomplete element results")
	}
	indices := make(map[ComponentID]int, len(records))
	for index, record := range records {
		indices[record.id] = index
	}
	checker := schemaSubstitutionCycleChecker{
		elements:  elements,
		indices:   indices,
		state:     make([]uint8, len(records)),
		pathNodes: make([]int, 0),
		pathEdges: make([]schemaElementSubstitutionGroup, 0),
		version:   version,
	}
	for index := range records {
		if checker.state[index] != 0 || len(elements[index].substitutionGroup) == 0 {
			continue
		}
		if err := checker.visit(index); err != nil {
			return err
		}
	}
	return nil
}

func (checker *schemaSubstitutionCycleChecker) visit(index int) error {
	if checker.state[index] == 2 {
		return nil
	}
	checker.state[index] = 1
	checker.pathNodes = append(checker.pathNodes, index)
	if err := checker.visitAffiliations(index); err != nil {
		return err
	}
	checker.pathNodes = checker.pathNodes[:len(checker.pathNodes)-1]
	checker.state[index] = 2
	return nil
}

func (checker *schemaSubstitutionCycleChecker) visitAffiliations(index int) error {
	for _, affiliation := range checker.elements[index].substitutionGroup {
		target, ok := checker.indices[affiliation.targetID]
		if !ok {
			return newSchemaBridgeInvariant(affiliation.loc, "substitution-group target ID is not allocated")
		}
		if checker.state[target] == 1 {
			return checker.cycleDiagnostic(target, affiliation)
		}
		if checker.state[target] != 0 {
			continue
		}
		checker.pathEdges = append(checker.pathEdges, affiliation)
		if err := checker.visit(target); err != nil {
			return err
		}
		checker.pathEdges = checker.pathEdges[:len(checker.pathEdges)-1]
	}
	return nil
}

func (checker *schemaSubstitutionCycleChecker) cycleDiagnostic(target int, affiliation schemaElementSubstitutionGroup) error {
	cycleStart := 0
	for position, node := range checker.pathNodes {
		if node == target {
			cycleStart = position
			break
		}
	}
	related := make([]Loc, 0, len(checker.pathEdges)-cycleStart)
	for _, edge := range checker.pathEdges[cycleStart:] {
		if edge.loc.IsZero() || edge.loc == affiliation.loc {
			continue
		}
		related = append(related, edge.loc)
	}
	return newSchemaSubstitutionGroupDiagnostic(
		diagnosticSchemaSubstitutionCycleCode,
		affiliation.loc,
		"substitutionGroup affiliations form a cycle",
		related,
		errSchemaSubstitutionCycle,
		schemaSubstitutionConstraintSpecRef(checker.version),
	)
}

//nolint:funlen,gocognit // Keep the ordered global element type branches together.
func resolveSchemaElementType(
	record schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes schemaSimpleTypeResolution,
	complexTypes []schemaComplexTypeResult,
	version XSDVersion,
) (schemaElementTypeResult, error) {
	input := record.element
	if input == nil {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(record.loc, "element type resolution has no type input")
	}
	if input.inlineSimpleType != nil {
		result, ok := simpleTypes.byInput[input.inlineSimpleType]
		if !ok || !result.present {
			return schemaElementTypeResult{}, newSchemaBridgeInvariant(
				input.typeLoc,
				"inline element simple type has no resolved result",
			)
		}
		if err := rejectUnsupportedSchemaSimpleTypeVariety(input, result, version, "for global elements"); err != nil {
			return schemaElementTypeResult{}, err
		}
		if !result.hasNodeID || result.nodeID.IsZero() {
			return schemaElementTypeResult{}, newSchemaBridgeInvariant(
				input.typeLoc,
				"inline element simple type has no allocated model identity",
			)
		}
		return schemaElementTypeResult{
			present:  true,
			abstract: input.abstract,
			nillable: input.nillable,
			block:    input.block,
			typeReference: schemaSimpleTypeReferenceComponent{
				kind:           SimpleTypeReferenceAnonymous,
				loc:            input.typeLoc,
				anonymousID:    result.nodeID,
				hasAnonymousID: true,
				anonymous:      schemaSimpleTypeComponentFromResult(result, true),
				variety:        result.variety,
				varietyLoc:     result.varietyLoc,
				atomicKind:     result.atomicKind,
				facets:         result.facets,
			},
			hasTypeReference: true,
		}, nil
	}
	if input.declaredType.Namespace() == xsdNamespaceURI {
		return resolveSchemaScalarType(input, records, byName, visibleSources, record.id.Source(), simpleTypes.results, version, "for global elements", schemaScalarTypeGlobalElement, true)
	}

	candidates := byName[input.declaredType]
	if len(candidates) == 0 {
		return unresolvedSchemaElementType(input, version)
	}
	visibleCandidates, err := schemaVisibleCandidates(candidates, record.id.Source(), records, visibleSources, input.typeLoc)
	if err != nil {
		return schemaElementTypeResult{}, err
	}
	if len(visibleCandidates) == 0 {
		return unresolvedSchemaElementType(input, version)
	}
	typeCandidates := make([]int, 0, len(visibleCandidates))
	for _, candidate := range visibleCandidates {
		kind := records[candidate].kind
		if kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
			continue
		}
		typeCandidates = append(typeCandidates, candidate)
	}
	if len(typeCandidates) > 1 {
		return ambiguousSchemaElementType(input, schemaComponentLocations(records, typeCandidates), version)
	}
	if len(typeCandidates) == 0 {
		return wrongKindSchemaElementType(input, schemaComponentLocations(records, visibleCandidates), version)
	}
	candidate := typeCandidates[0]
	if records[candidate].kind == ComponentKindComplexTypeDefinition {
		if !complexTypes[candidate].present || schemaComplexTypeResultIsEmpty(complexTypes[candidate]) {
			return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
				input.typeLoc,
				fmt.Sprintf("named complex type %q is not implemented for global elements", input.declaredType),
				version,
			)
		}
		return resolvedSchemaElementTypeResult(input, records[candidate].id, true), nil
	}
	if !simpleTypes.results[candidate].present {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(
			input.typeLoc,
			"element type resolution has an incomplete simple type result",
		)
	}
	if err := rejectUnsupportedSchemaSimpleTypeVariety(input, simpleTypes.results[candidate], version, "for global elements"); err != nil {
		return schemaElementTypeResult{}, err
	}
	return schemaElementTypeResult{
		present:      true,
		declaredType: input.declaredType,
		typeID:       records[candidate].id,
		hasTypeID:    true,
		typeReference: schemaSimpleTypeReferenceComponent{
			kind:       SimpleTypeReferenceNamed,
			name:       input.declaredType,
			loc:        input.typeLoc,
			id:         records[candidate].id,
			hasID:      true,
			variety:    simpleTypes.results[candidate].variety,
			varietyLoc: simpleTypes.results[candidate].varietyLoc,
			atomicKind: simpleTypes.results[candidate].atomicKind,
			facets:     simpleTypes.results[candidate].facets,
		},
		hasTypeReference: true,
		abstract:         input.abstract,
		nillable:         input.nillable,
		block:            input.block,
	}, nil
}

func schemaComplexTypeResultIsEmpty(result schemaComplexTypeResult) bool {
	if !result.present || result.body == nil {
		return false
	}
	_, ok := result.body.(*schemaComplexTypeEmptyBodyResult)
	return ok
}

func resolveSchemaScalarType(
	input *schemaElementInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	referringSource SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
	complexTargetSuffix string,
	scope schemaScalarTypeScope,
	allowPrecisionDecimal bool,
) (schemaElementTypeResult, error) {
	if input.declaredType.Namespace() == xsdNamespaceURI {
		return resolveBuiltinSchemaScalarType(input, version, complexTargetSuffix, scope, allowPrecisionDecimal)
	}

	candidates := byName[input.declaredType]
	if len(candidates) == 0 {
		return unresolvedSchemaElementType(input, version)
	}
	visibleCandidates, err := schemaVisibleCandidates(candidates, referringSource, records, visibleSources, input.typeLoc)
	if err != nil {
		return schemaElementTypeResult{}, err
	}
	if len(visibleCandidates) == 0 {
		return unresolvedSchemaElementType(input, version)
	}
	typeCandidates := make([]int, 0, len(candidates))
	for _, candidate := range visibleCandidates {
		kind := records[candidate].kind
		if kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
			continue
		}
		typeCandidates = append(typeCandidates, candidate)
	}
	if len(typeCandidates) > 1 {
		return ambiguousSchemaElementType(input, schemaComponentLocations(records, typeCandidates), version)
	}
	if len(typeCandidates) == 0 {
		return wrongKindSchemaElementType(input, schemaComponentLocations(records, visibleCandidates), version)
	}
	candidate := typeCandidates[0]
	if records[candidate].kind == ComponentKindComplexTypeDefinition {
		return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
			input.typeLoc,
			fmt.Sprintf("named complex type %q is not implemented %s", input.declaredType, complexTargetSuffix),
			version,
		)
	}
	if !simpleTypes[candidate].present {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(
			input.typeLoc,
			"element type resolution has an incomplete simple type result",
		)
	}
	if err := rejectUnsupportedSchemaSimpleTypeVariety(input, simpleTypes[candidate], version, complexTargetSuffix); err != nil {
		return schemaElementTypeResult{}, err
	}
	if err := rejectUnsupportedLocalScalarType(input, simpleTypes[candidate], version, complexTargetSuffix, scope, allowPrecisionDecimal); err != nil {
		return schemaElementTypeResult{}, err
	}
	return resolvedSchemaElementTypeResult(input, records[candidate].id, true), nil
}

func rejectUnsupportedSchemaSimpleTypeVariety(input *schemaElementInput, simpleType schemaSimpleTypeResult, version XSDVersion, context string) error {
	if simpleType.variety == SimpleTypeVarietyAtomicRestriction {
		return nil
	}
	message := fmt.Sprintf("element type %q has simple type variety %q, which is not implemented %s", input.declaredType, simpleType.variety, context)
	if input.declaredType.IsZero() {
		message = fmt.Sprintf("inline element simple type has variety %q, which is not implemented %s", simpleType.variety, context)
	}
	return newSchemaSyntaxUnsupportedForVersion(
		input.typeLoc,
		message,
		version,
	)
}

//nolint:gocognit // Keep built-in scalar scope and version branches explicit.
func resolveBuiltinSchemaScalarType(input *schemaElementInput, version XSDVersion, complexTargetSuffix string, scope schemaScalarTypeScope, allowPrecisionDecimal bool) (schemaElementTypeResult, error) {
	switch input.declaredType.Local() {
	case "string", "token":
		if scope != schemaScalarTypeGlobalElement {
			return schemaElementTypeResult{}, unsupportedLocalSchemaScalarType(input, version, complexTargetSuffix)
		}
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
			abstract:         input.abstract,
			nillable:         input.nillable,
			block:            input.block,
		}, nil
	case "integer", "decimal":
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
			abstract:         input.abstract,
			nillable:         input.nillable,
			block:            input.block,
		}, nil
	case "boolean":
		if !scope.allowsBoolean() {
			return schemaElementTypeResult{}, unsupportedLocalSchemaScalarType(input, version, complexTargetSuffix)
		}
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
			abstract:         input.abstract,
			nillable:         input.nillable,
			block:            input.block,
		}, nil
	case "precisionDecimal":
		if version == XSDVersion10 {
			return schemaElementTypeResult{}, precisionDecimalSchemaVersionDiagnostic(input.typeLoc, input.declaredType)
		}
		if !allowPrecisionDecimal {
			return schemaElementTypeResult{}, unsupportedSequencePrecisionDecimal(input, version)
		}
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
			abstract:         input.abstract,
			nillable:         input.nillable,
			block:            input.block,
		}, nil
	default:
		return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
			input.typeLoc,
			fmt.Sprintf("element type %q is not implemented", input.declaredType),
			version,
		)
	}
}

func builtinSchemaElementTypeReference(input *schemaElementInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	return resolveBuiltinSchemaSimpleTypeReference(schemaSimpleTypeReferenceInput{
		kind: schemaSimpleTypeQNameReferenceInput,
		name: input.declaredType,
		loc:  input.typeLoc,
	}, version)
}

func rejectUnsupportedLocalScalarType(input *schemaElementInput, simpleType schemaSimpleTypeResult, version XSDVersion, complexTargetSuffix string, scope schemaScalarTypeScope, allowPrecisionDecimal bool) error {
	if scope != schemaScalarTypeLocalParticle {
		return nil
	}
	switch simpleType.atomicKind {
	case schemaSimpleTypeAtomicString,
		schemaSimpleTypeAtomicToken,
		schemaSimpleTypeAtomicLanguage,
		schemaSimpleTypeAtomicNCName,
		schemaSimpleTypeAtomicAnyURI,
		schemaSimpleTypeAtomicID:
		return unsupportedLocalSchemaScalarType(input, version, complexTargetSuffix)
	case schemaSimpleTypeAtomicUnknown,
		schemaSimpleTypeAtomicInteger,
		schemaSimpleTypeAtomicNegativeInteger,
		schemaSimpleTypeAtomicDecimal,
		schemaSimpleTypeAtomicPrecisionDecimal:
		break
	}
	if allowPrecisionDecimal {
		return nil
	}
	if _, ok := simpleType.facets.(schemaPrecisionDecimalFacetVariant); !ok {
		return nil
	}
	return unsupportedSequencePrecisionDecimal(input, version)
}

func unsupportedLocalSchemaScalarType(input *schemaElementInput, version XSDVersion, complexTargetSuffix string) Diagnostic {
	return newSchemaSyntaxUnsupportedForVersion(
		input.typeLoc,
		fmt.Sprintf("element type %q is not implemented %s", input.declaredType, complexTargetSuffix),
		version,
	)
}

func unsupportedSequencePrecisionDecimal(input *schemaElementInput, version XSDVersion) Diagnostic {
	return newSchemaSyntaxUnsupportedForVersion(
		input.typeLoc,
		fmt.Sprintf("element type %q is not implemented for local sequence elements", input.declaredType),
		version,
	)
}

type schemaComplexTypeResult struct {
	present                 bool
	body                    schemaComplexTypeBodyResult
	prohibitedSubstitutions schemaBlockPolicy
}

type schemaComplexTypeBodyResult interface {
	schemaComplexTypeBodyResult()
}

type schemaComplexTypeDirectBodyResult struct {
	particle     Particle
	anyAttribute schemaAnyAttributeResult
}

func (*schemaComplexTypeDirectBodyResult) schemaComplexTypeBodyResult() {}

type schemaComplexTypeEmptyBodyResult struct {
	anyAttribute schemaAnyAttributeResult
}

func (*schemaComplexTypeEmptyBodyResult) schemaComplexTypeBodyResult() {}

type schemaComplexTypeRestrictionBodyResult struct {
	complexContentLoc Loc
	restrictionLoc    Loc
	base              schemaComplexTypeReferenceComponent
	anyAttribute      schemaAnyAttributeResult
}

func (*schemaComplexTypeRestrictionBodyResult) schemaComplexTypeBodyResult() {}

type schemaComplexTypeExtensionBodyResult struct {
	complexContentLoc Loc
	extensionLoc      Loc
	base              schemaComplexTypeReferenceComponent
	particle          Particle
	anyAttribute      schemaAnyAttributeResult
}

func (*schemaComplexTypeExtensionBodyResult) schemaComplexTypeBodyResult() {}

type schemaAnyAttributeResult struct {
	present            bool
	loc                Loc
	namespace          string
	namespaceLoc       Loc
	processContents    string
	processContentsLoc Loc
}

type schemaModelGroupResult struct {
	present  bool
	particle Particle
}

func resolveSchemaComplexTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) ([]schemaComplexTypeResult, error) {
	if len(simpleTypes) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "complex type resolution has incomplete simple type results")
	}
	resolver := schemaComplexTypeResolver{
		records:        records,
		byName:         byName,
		visibleSources: visibleSources,
		simpleTypes:    simpleTypes,
		version:        version,
		results:        make([]schemaComplexTypeResult, len(records)),
		state:          make([]uint8, len(records)),
		stack:          make([]int, 0),
	}
	for index, record := range records {
		if record.complexType == nil {
			continue
		}
		if err := resolver.resolve(index); err != nil {
			return nil, err
		}
	}
	return resolver.results, nil
}

type schemaComplexTypeResolver struct {
	records        []schemaComponentRecord
	byName         map[QName][]int
	visibleSources map[SourceID][]SourceID
	simpleTypes    []schemaSimpleTypeResult
	version        XSDVersion
	results        []schemaComplexTypeResult
	state          []uint8
	stack          []int
}

func (resolver *schemaComplexTypeResolver) resolve(index int) error {
	if index < 0 || index >= len(resolver.records) {
		return newSchemaBridgeInvariant(Loc{}, "complex type resolution has an invalid record index")
	}
	if resolver.records[index].complexType == nil {
		return newSchemaBridgeInvariant(resolver.records[index].loc, "complex type resolution has no input")
	}
	if resolver.state[index] == 2 {
		return nil
	}
	if resolver.state[index] == 1 {
		return resolver.cycleDiagnostic(index, resolver.records[index].loc)
	}
	resolver.state[index] = 1
	resolver.stack = append(resolver.stack, index)
	record := resolver.records[index]
	if record.complexType.body == nil {
		return newSchemaBridgeInvariant(record.loc, "complex type resolution has no body input")
	}
	body, err := resolver.resolveBody(record.complexType.body, index)
	if err != nil {
		return err
	}
	resolver.stack = resolver.stack[:len(resolver.stack)-1]
	resolver.state[index] = 2
	resolver.results[index] = schemaComplexTypeResult{
		present:                 true,
		body:                    body,
		prohibitedSubstitutions: record.complexType.prohibitedSubstitutions,
	}
	return nil
}

//nolint:gocognit // Keep the phase-specific body variants explicit.
func (resolver *schemaComplexTypeResolver) resolveBody(
	input schemaComplexTypeBodyInput,
	ownerIndex int,
) (schemaComplexTypeBodyResult, error) {
	owner := resolver.records[ownerIndex]
	switch body := input.(type) {
	case *schemaComplexTypeDirectBodyInput:
		if body == nil || body.particle == nil {
			return nil, newSchemaBridgeInvariant(owner.loc, "direct complex type body has no particle input")
		}
		particle, err := resolveSchemaComplexTypeParticle(
			body.particle,
			owner,
			resolver.records,
			resolver.byName,
			resolver.visibleSources,
			resolver.simpleTypes,
			resolver.version,
		)
		if err != nil {
			return nil, err
		}
		anyAttribute := schemaAnyAttributeResultFromInput(body.anyAttribute)
		if particle == nil {
			return &schemaComplexTypeEmptyBodyResult{anyAttribute: anyAttribute}, nil
		}
		return &schemaComplexTypeDirectBodyResult{
			particle:     particle,
			anyAttribute: anyAttribute,
		}, nil
	case *schemaComplexTypeEmptyBodyInput:
		if body == nil {
			return nil, newSchemaBridgeInvariant(owner.loc, "empty complex type body is nil")
		}
		return &schemaComplexTypeEmptyBodyResult{}, nil
	case *schemaComplexTypeRestrictionBodyInput:
		if body == nil {
			return nil, newSchemaBridgeInvariant(owner.loc, "restriction complex type body is nil")
		}
		base, err := resolveSchemaComplexTypeReference(body.base, owner, resolver.records, resolver.byName, resolver.visibleSources, resolver.version)
		if err != nil {
			return nil, err
		}
		if body.anyAttribute == nil {
			return nil, newSchemaBridgeInvariant(owner.loc, "restriction complex type body has no wildcard input")
		}
		return &schemaComplexTypeRestrictionBodyResult{
			complexContentLoc: body.complexContentLoc,
			restrictionLoc:    body.restrictionLoc,
			base:              base,
			anyAttribute:      schemaAnyAttributeResultFromInput(body.anyAttribute),
		}, nil
	case *schemaComplexTypeExtensionBodyInput:
		if body == nil || body.particle == nil {
			return nil, newSchemaBridgeInvariant(owner.loc, "extension complex type body has no particle input")
		}
		particle, err := resolveSchemaComplexTypeParticle(
			body.particle,
			owner,
			resolver.records,
			resolver.byName,
			resolver.visibleSources,
			resolver.simpleTypes,
			resolver.version,
		)
		if err != nil {
			return nil, err
		}
		base, anyAttribute, err := resolver.resolveExtensionBase(body.base, ownerIndex)
		if err != nil {
			return nil, err
		}
		return &schemaComplexTypeExtensionBodyResult{
			complexContentLoc: body.complexContentLoc,
			extensionLoc:      body.extensionLoc,
			base:              base,
			particle:          particle,
			anyAttribute:      anyAttribute,
		}, nil
	default:
		return nil, newSchemaBridgeInvariant(owner.loc, "complex type body has an unknown input variant")
	}
}

func (resolver *schemaComplexTypeResolver) resolveExtensionBase(
	input schemaComplexTypeReferenceInput,
	ownerIndex int,
) (schemaComplexTypeReferenceComponent, schemaAnyAttributeResult, error) {
	owner := resolver.records[ownerIndex]
	candidate, reference, err := resolveSchemaComplexTypeExtensionReference(
		input,
		owner,
		resolver.records,
		resolver.byName,
		resolver.visibleSources,
		resolver.version,
	)
	if err != nil {
		return schemaComplexTypeReferenceComponent{}, schemaAnyAttributeResult{}, err
	}
	if resolver.state[candidate] == 1 {
		return schemaComplexTypeReferenceComponent{}, schemaAnyAttributeResult{}, resolver.cycleDiagnostic(candidate, input.loc)
	}
	if resolveErr := resolver.resolve(candidate); resolveErr != nil {
		return schemaComplexTypeReferenceComponent{}, schemaAnyAttributeResult{}, resolveErr
	}
	base := resolver.results[candidate]
	if !base.present || base.body == nil {
		return schemaComplexTypeReferenceComponent{}, schemaAnyAttributeResult{}, newSchemaBridgeInvariant(
			input.loc,
			"extension base has no completed complex type result",
		)
	}
	inherited, err := resolver.extensionBaseFacts(base.body, resolver.records[candidate].loc, input.loc)
	if err != nil {
		return schemaComplexTypeReferenceComponent{}, schemaAnyAttributeResult{}, err
	}
	reference.id = resolver.records[candidate].id
	reference.hasID = true
	return reference, inherited, nil
}

func (resolver *schemaComplexTypeResolver) extensionBaseFacts(
	body schemaComplexTypeBodyResult,
	baseLoc Loc,
	baseReferenceLoc Loc,
) (schemaAnyAttributeResult, error) {
	switch typed := body.(type) {
	case *schemaComplexTypeEmptyBodyResult:
		if typed == nil {
			return schemaAnyAttributeResult{}, newSchemaBridgeInvariant(baseLoc, "empty extension base result is nil")
		}
		return resolver.representableInheritedWildcard(typed.anyAttribute, baseLoc, baseReferenceLoc)
	case *schemaComplexTypeRestrictionBodyResult:
		if typed == nil {
			return schemaAnyAttributeResult{}, newSchemaBridgeInvariant(baseLoc, "restriction extension base result is nil")
		}
		if typed.base.kind != ComplexTypeReferenceBuiltin || typed.base.name.Namespace() != xsdNamespaceURI || typed.base.name.Local() != "anyType" {
			return schemaAnyAttributeResult{}, resolver.unsupportedExtensionBase(
				baseReferenceLoc,
				"named complex type extension base has unsupported restriction composition",
				[]Loc{baseLoc, typed.restrictionLoc},
				fmt.Errorf("%w: restriction composition", errSchemaComplexTypeBaseUnsupported),
			)
		}
		return resolver.representableInheritedWildcard(typed.anyAttribute, baseLoc, baseReferenceLoc)
	case *schemaComplexTypeDirectBodyResult:
		if typed == nil {
			return schemaAnyAttributeResult{}, newSchemaBridgeInvariant(baseLoc, "direct extension base result is nil")
		}
		related := []Loc{baseLoc, typed.particle.Loc()}
		return schemaAnyAttributeResult{}, resolver.unsupportedExtensionBase(
			baseReferenceLoc,
			"named complex type extension base has nonempty content",
			related,
			fmt.Errorf("%w: %w", errSchemaComplexTypeBaseUnsupported, errSchemaComplexTypeBaseNonEmpty),
		)
	case *schemaComplexTypeExtensionBodyResult:
		if typed == nil {
			return schemaAnyAttributeResult{}, newSchemaBridgeInvariant(baseLoc, "extension extension base result is nil")
		}
		related := []Loc{baseLoc, typed.extensionLoc}
		if !typed.particleIsNil() {
			related = append(related, typed.particleLoc())
		}
		return schemaAnyAttributeResult{}, resolver.unsupportedExtensionBase(
			baseReferenceLoc,
			"named complex type extension base has unsupported extension composition",
			related,
			fmt.Errorf("%w: extension composition", errSchemaComplexTypeBaseUnsupported),
		)
	default:
		return schemaAnyAttributeResult{}, newSchemaBridgeInvariant(baseLoc, "extension base has an unknown completed body")
	}
}

func (resolver *schemaComplexTypeResolver) representableInheritedWildcard(
	wildcard schemaAnyAttributeResult,
	baseLoc Loc,
	baseReferenceLoc Loc,
) (schemaAnyAttributeResult, error) {
	if !wildcard.present {
		return schemaAnyAttributeResult{}, nil
	}
	if wildcard.namespace == "##other" && wildcard.processContents == "lax" {
		return wildcard, nil
	}
	return schemaAnyAttributeResult{}, resolver.unsupportedExtensionBase(
		baseReferenceLoc,
		"named complex type extension base has an unrepresentable attribute wildcard",
		[]Loc{baseLoc, wildcard.loc},
		fmt.Errorf("%w: attribute wildcard", errSchemaComplexTypeBaseUnsupported),
	)
}

func (resolver *schemaComplexTypeResolver) unsupportedExtensionBase(loc Loc, message string, related []Loc, cause error) error {
	return newSchemaComplexTypeUnsupportedWithSpec(
		loc,
		message,
		related,
		resolver.version,
		cause,
		schemaComplexTypeExtensionSpecRef(resolver.version),
	)
}

func (resolver *schemaComplexTypeResolver) cycleDiagnostic(target int, edgeLoc Loc) error {
	start := 0
	for index, node := range resolver.stack {
		if node == target {
			start = index
			break
		}
	}
	related := make([]Loc, 0, len(resolver.stack)-start+1)
	for _, node := range resolver.stack[start:] {
		body, ok := resolver.records[node].complexType.body.(*schemaComplexTypeExtensionBodyInput)
		if !ok || body == nil || body.base.loc.IsZero() || body.base.loc == edgeLoc {
			continue
		}
		related = append(related, body.base.loc)
	}
	return newSchemaComplexTypeExtensionBaseDiagnostic(
		edgeLoc,
		"named complex type extension bases form a cycle",
		related,
		resolver.version,
		errSchemaComplexTypeBaseCycle,
	)
}

func (body *schemaComplexTypeExtensionBodyResult) particleIsNil() bool {
	return body == nil || body.particle == nil
}

func (body *schemaComplexTypeExtensionBodyResult) particleLoc() Loc {
	if body == nil || body.particle == nil {
		return Loc{}
	}
	return body.particle.Loc()
}

func schemaAnyAttributeResultFromInput(input *schemaAnyAttributeInput) schemaAnyAttributeResult {
	if input == nil {
		return schemaAnyAttributeResult{}
	}
	return schemaAnyAttributeResult{
		present:            true,
		loc:                input.loc,
		namespace:          input.namespace,
		namespaceLoc:       input.namespaceLoc,
		processContents:    input.processContents,
		processContentsLoc: input.processContentsLoc,
	}
}

//nolint:funlen // Keep the complete base-kind and visibility classification together.
func resolveSchemaComplexTypeReference(
	input schemaComplexTypeReferenceInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	version XSDVersion,
) (schemaComplexTypeReferenceComponent, error) {
	if input.kind != schemaComplexTypeQNameReferenceInput {
		return schemaComplexTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "complex type base reference has an unknown kind")
	}
	if input.name.IsZero() {
		return schemaComplexTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "complex type base reference is empty")
	}
	if input.name.Namespace() == xsdNamespaceURI {
		if input.name.Local() == "anyType" {
			return schemaComplexTypeReferenceComponent{
				kind: ComplexTypeReferenceBuiltin,
				name: input.name,
				loc:  input.loc,
			}, nil
		}
		return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type restriction base %q is not the supported xs:anyType complex base", input.name),
			nil,
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseWrongKind, input.name),
		)
	}

	candidates := byName[input.name]
	if len(candidates) == 0 {
		return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type restriction base %q cannot be resolved", input.name),
			nil,
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseUnresolved, input.name),
		)
	}
	visibleCandidates, err := schemaVisibleCandidates(candidates, owner.id.Source(), records, visibleSources, input.loc)
	if err != nil {
		return schemaComplexTypeReferenceComponent{}, err
	}
	if len(visibleCandidates) == 0 {
		return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type restriction base %q cannot be resolved", input.name),
			nil,
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseUnresolved, input.name),
		)
	}
	typeCandidates := make([]int, 0, len(visibleCandidates))
	for _, candidate := range visibleCandidates {
		kind := records[candidate].kind
		if kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
			continue
		}
		typeCandidates = append(typeCandidates, candidate)
	}
	if len(typeCandidates) > 1 {
		return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type restriction base %q is ambiguous", input.name),
			schemaComponentLocations(records, typeCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseAmbiguous, input.name),
		)
	}
	if len(typeCandidates) == 0 {
		return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type restriction base %q does not name a type definition", input.name),
			schemaComponentLocations(records, visibleCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseWrongKind, input.name),
		)
	}
	candidate := typeCandidates[0]
	if records[candidate].kind == ComponentKindComplexTypeDefinition {
		return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeUnsupported(
			input.loc,
			fmt.Sprintf("named complex type restriction base %q is not implemented", input.name),
			[]Loc{records[candidate].loc},
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseUnsupported, input.name),
		)
	}
	return schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeBaseDiagnostic(
		input.loc,
		fmt.Sprintf("complex type restriction base %q names a simple type", input.name),
		[]Loc{records[candidate].loc},
		version,
		fmt.Errorf("%w: %q", errSchemaComplexTypeBaseWrongKind, input.name),
	)
}

// resolveSchemaComplexTypeExtensionReference classifies the named base before
// the resolver follows its completed complex-type result. Component IDs are
// already allocated by the caller's declaration phase.
//
//nolint:gocognit,funlen // Keep visibility, kind, and representation precedence together.
func resolveSchemaComplexTypeExtensionReference(
	input schemaComplexTypeReferenceInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	version XSDVersion,
) (int, schemaComplexTypeReferenceComponent, error) {
	if input.kind != schemaComplexTypeQNameReferenceInput {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "complex type extension base reference has an unknown kind")
	}
	if input.name.IsZero() {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "complex type extension base reference is empty")
	}
	if input.name.Namespace() == xsdNamespaceURI {
		if input.name.Local() == "anyType" {
			return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeUnsupportedWithSpec(
				input.loc,
				fmt.Sprintf("complex type extension base %q is not a named complex type", input.name),
				nil,
				version,
				errSchemaComplexTypeBaseUnsupported,
				schemaComplexTypeExtensionSpecRef(version),
			)
		}
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeExtensionBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type extension base %q is not a supported complex type", input.name),
			nil,
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseWrongKind, input.name),
		)
	}

	candidates := byName[input.name]
	if len(candidates) == 0 {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeExtensionBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type extension base %q cannot be resolved", input.name),
			nil,
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseUnresolved, input.name),
		)
	}
	visibleCandidates, err := schemaVisibleCandidates(candidates, owner.id.Source(), records, visibleSources, input.loc)
	if err != nil {
		return 0, schemaComplexTypeReferenceComponent{}, err
	}
	if len(visibleCandidates) == 0 {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeExtensionBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type extension base %q is not visible from its schema document", input.name),
			schemaComponentLocations(records, candidates),
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseUnresolved, input.name),
		)
	}
	typeCandidates := make([]int, 0, len(visibleCandidates))
	for _, candidate := range visibleCandidates {
		kind := records[candidate].kind
		if kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
			continue
		}
		typeCandidates = append(typeCandidates, candidate)
	}
	if len(typeCandidates) > 1 {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeExtensionBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type extension base %q is ambiguous", input.name),
			schemaComponentLocations(records, typeCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseAmbiguous, input.name),
		)
	}
	if len(typeCandidates) == 0 {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeExtensionBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type extension base %q does not name a type definition", input.name),
			schemaComponentLocations(records, visibleCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseWrongKind, input.name),
		)
	}
	candidate := typeCandidates[0]
	if records[candidate].kind != ComponentKindComplexTypeDefinition {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeExtensionBaseDiagnostic(
			input.loc,
			fmt.Sprintf("complex type extension base %q names a simple type", input.name),
			[]Loc{records[candidate].loc},
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseWrongKind, input.name),
		)
	}
	if records[candidate].complexType == nil {
		return 0, schemaComplexTypeReferenceComponent{}, newSchemaComplexTypeUnsupportedWithSpec(
			input.loc,
			fmt.Sprintf("complex type extension base %q has unsupported content", input.name),
			[]Loc{records[candidate].loc},
			version,
			fmt.Errorf("%w: %q", errSchemaComplexTypeBaseUnsupported, input.name),
			schemaComplexTypeExtensionSpecRef(version),
		)
	}
	return candidate, schemaComplexTypeReferenceComponent{
		kind: ComplexTypeReferenceNamed,
		name: input.name,
		loc:  input.loc,
	}, nil
}

func newSchemaComplexTypeBaseDiagnostic(loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    invalidSchemaCompositionCode,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaComplexTypeDerivationSpecRef(version),
		cause:   cause,
	}
}

func newSchemaComplexTypeUnsupported(loc Loc, message string, related []Loc, version XSDVersion, cause error) error {
	return newSchemaComplexTypeUnsupportedWithSpec(loc, message, related, version, cause, schemaComplexTypeDerivationSpecRef(version))
}

func newSchemaComplexTypeExtensionBaseDiagnostic(loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    invalidSchemaCompositionCode,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaComplexTypeExtensionSpecRef(version),
		cause:   cause,
	}
}

func newSchemaComplexTypeUnsupportedWithSpec(loc Loc, message string, related []Loc, version XSDVersion, cause error, specRef string) error {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxFeatureCode,
			loc,
			"schema syntax feature is not registered",
			cause,
		)
	}
	diagnostic := newUnsupportedForVersionWithCause(
		feature,
		UnsupportedSchemaSyntaxCode,
		loc,
		message,
		version,
		cause,
	)
	if diagnostic.Class() == FailureUnsupported {
		diagnostic.related = append([]Loc(nil), related...)
		diagnostic.specRef = specRef
	}
	return diagnostic
}

func schemaComplexTypeDerivationSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaComplexTypeDerivationXSD10SpecRef
	}
	return schemaComplexTypeDerivationXSD11SpecRef
}

func schemaComplexContentExtensionSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaComplexContentExtensionXSD10SpecRef
	}
	return schemaComplexContentExtensionXSD11SpecRef
}

func schemaComplexTypeExtensionSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaComplexTypeExtensionXSD10SpecRef
	}
	return schemaComplexTypeExtensionXSD11SpecRef
}

func schemaComplexParticleExtensionSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaComplexParticleExtensionXSD10SpecRef
	}
	return schemaComplexParticleExtensionXSD11SpecRef
}

func resolveSchemaModelGroups(
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) ([]schemaModelGroupResult, error) {
	if len(simpleTypes) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "model group resolution has incomplete simple type results")
	}
	results := make([]schemaModelGroupResult, len(records))
	for index, record := range records {
		if record.modelGroup == nil {
			continue
		}
		if record.modelGroup.particle == nil {
			return nil, newSchemaBridgeInvariant(record.loc, "model group resolution has no particle input")
		}
		particle, err := resolveSchemaChoiceParticleWithOptions(
			record.modelGroup.particle,
			record,
			records,
			byName,
			visibleSources,
			simpleTypes,
			version,
			true,
		)
		if err != nil {
			return nil, err
		}
		results[index] = schemaModelGroupResult{
			present:  true,
			particle: particle,
		}
	}
	return results, nil
}

func resolveSchemaComplexTypeParticle(
	input schemaComplexTypeParticleInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) (Particle, error) {
	switch particle := input.(type) {
	case *schemaChoiceParticleInput:
		if particle == nil {
			return nil, newSchemaBridgeInvariant(Loc{}, "choice particle input is nil")
		}
		return resolveSchemaChoiceParticle(particle, owner, records, byName, visibleSources, simpleTypes, version)
	case *schemaSequenceParticleInput:
		if particle == nil {
			return nil, newSchemaBridgeInvariant(Loc{}, "sequence particle input is nil")
		}
		return resolveSchemaSequenceParticle(particle, owner, records, byName, visibleSources, simpleTypes, version)
	default:
		return nil, newSchemaBridgeInvariant(Loc{}, "complex type has an unknown particle input")
	}
}

func resolveSchemaChoiceParticle(
	input *schemaChoiceParticleInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) (Particle, error) {
	return resolveSchemaChoiceParticleWithOptions(
		input,
		owner,
		records,
		byName,
		visibleSources,
		simpleTypes,
		version,
		false,
	)
}

//nolint:gocognit // Keep occurrence-sensitive choice resolution and duplicate checks together.
func resolveSchemaChoiceParticleWithOptions(
	input *schemaChoiceParticleInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
	rejectDuplicateReferences bool,
) (Particle, error) {
	mapsToParticle := input.occurrences.mapsToParticle()
	alternatives := make([]Particle, 0, len(input.alternatives))
	seenReferences := make(map[QName]Loc)
	for _, elementInput := range input.alternatives {
		if !mapsToParticle && elementInput.reference == nil {
			continue
		}
		if !input.occurrences.isDefault() && elementInput.occurrences.mapsToParticle() && elementInput.typeInput != nil {
			isPrecisionDecimal, err := schemaScalarTypeIsPrecisionDecimal(
				elementInput.typeInput.declaredType,
				records,
				byName,
				owner.id.Source(),
				visibleSources,
				simpleTypes,
				elementInput.loc,
				version,
			)
			if err != nil {
				return nil, err
			}
			if isPrecisionDecimal {
				return nil, unsupportedChoicePrecisionDecimalParticle(input, version)
			}
		}
		element, err := resolveSchemaElementParticle(
			elementInput,
			owner,
			records,
			byName,
			visibleSources,
			simpleTypes,
			version,
			"choice",
		)
		if err != nil {
			return nil, err
		}
		if rejectDuplicateReferences && elementInput.reference != nil {
			firstLoc, seen := seenReferences[elementInput.reference.name]
			if seen {
				return nil, newSchemaElementReferenceDuplicateDiagnostic(
					elementInput.reference,
					firstLoc,
					version,
				)
			}
			seenReferences[elementInput.reference.name] = elementInput.reference.loc
		}
		if element == nil || !elementInput.occurrences.mapsToParticle() {
			continue
		}
		alternatives = append(alternatives, element)
	}
	if !mapsToParticle {
		return nil, nil
	}
	choice := &schemaChoiceParticle{
		loc:          input.loc,
		occurrences:  input.occurrences.clone(),
		alternatives: alternatives,
	}
	return ChoiceParticle{facts: choice}, nil
}

func resolveSchemaSequenceParticle(
	input *schemaSequenceParticleInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) (Particle, error) {
	if !input.occurrences.mapsToParticle() {
		return nil, nil
	}
	particles := make([]Particle, 0, len(input.elements))
	for _, elementInput := range input.elements {
		element, err := resolveSchemaElementParticle(
			elementInput,
			owner,
			records,
			byName,
			visibleSources,
			simpleTypes,
			version,
			"sequence",
		)
		if err != nil {
			return nil, err
		}
		if element == nil {
			continue
		}
		particles = append(particles, element)
	}
	sequence := &schemaSequenceParticle{
		loc:         input.loc,
		occurrences: input.occurrences.clone(),
		particles:   particles,
	}
	return SequenceParticle{facts: sequence}, nil
}

func resolveSchemaElementParticle(
	input schemaElementParticleInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
	model string,
) (Particle, error) {
	if input.reference != nil {
		element, err := resolveSchemaElementReferenceParticle(input, owner, records, byName, visibleSources, version)
		if err != nil {
			return nil, err
		}
		if !input.occurrences.mapsToParticle() {
			return nil, nil
		}
		return element, nil
	}
	if !input.occurrences.mapsToParticle() {
		return nil, nil
	}
	if model == "choice" && !input.occurrences.isDefault() && input.typeInput != nil {
		isPrecisionDecimal, err := schemaScalarTypeIsPrecisionDecimal(
			input.typeInput.declaredType,
			records,
			byName,
			owner.id.Source(),
			visibleSources,
			simpleTypes,
			input.loc,
			version,
		)
		if err != nil {
			return nil, err
		}
		if isPrecisionDecimal {
			return nil, unsupportedChoicePrecisionDecimalAlternative(input, version)
		}
	}
	if input.typeInput == nil {
		return nil, newSchemaSyntaxUnsupported(
			input.loc,
			fmt.Sprintf("local %s elements without declared types are not implemented", model),
		)
	}
	resolved, err := resolveSchemaScalarType(
		input.typeInput,
		records,
		byName,
		visibleSources,
		owner.id.Source(),
		simpleTypes,
		version,
		"for local "+model+" elements",
		schemaScalarTypeLocalParticle,
		model != "sequence",
	)
	if err != nil {
		return nil, err
	}
	facts := &schemaElementParticle{
		loc:                     input.loc,
		occurrences:             input.occurrences.clone(),
		name:                    input.name,
		declaredType:            resolved.declaredType,
		nillable:                input.nillable,
		disallowedSubstitutions: input.block,
		typeID:                  resolved.typeID,
		hasTypeID:               resolved.hasTypeID,
	}
	return ElementParticle{facts: facts}, nil
}

func resolveSchemaElementReferenceParticle(
	input schemaElementParticleInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	visibleSources map[SourceID][]SourceID,
	version XSDVersion,
) (Particle, error) {
	if input.reference == nil {
		return nil, newSchemaBridgeInvariant(input.loc, "element reference input is nil")
	}
	reference := input.reference
	candidates := byName[reference.name]
	if len(candidates) == 0 {
		return nil, newSchemaElementReferenceDiagnostic(
			diagnosticSchemaElementReferenceUnresolvedCode,
			reference.loc,
			fmt.Sprintf("element reference %q is unresolved", reference.name),
			nil,
			version,
			errSchemaElementReferenceUnresolved,
		)
	}
	elementCandidates, err := schemaElementReferenceElementCandidates(candidates, records, reference.loc)
	if err != nil {
		return nil, err
	}
	if len(elementCandidates) == 0 {
		return nil, newSchemaElementReferenceDiagnostic(
			diagnosticSchemaElementReferenceWrongKindCode,
			reference.loc,
			fmt.Sprintf("element reference %q does not name a global element declaration", reference.name),
			schemaComponentLocations(records, candidates),
			version,
			errSchemaElementReferenceWrongKind,
		)
	}
	visible, err := schemaElementReferenceVisibleCandidates(elementCandidates, owner, records, visibleSources)
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return nil, schemaElementReferenceVisibilityDiagnostic(reference, owner, records, elementCandidates, version)
	}
	if len(visible) > 1 {
		return nil, newSchemaElementReferenceDiagnostic(
			diagnosticSchemaElementReferenceAmbiguousCode,
			reference.loc,
			fmt.Sprintf("element reference %q is ambiguous", reference.name),
			schemaComponentLocations(records, visible),
			version,
			errSchemaElementReferenceAmbiguous,
		)
	}
	facts := &schemaElementReferenceParticle{
		loc:         input.loc,
		occurrences: input.occurrences.clone(),
		name:        reference.name,
		refLoc:      reference.loc,
		targetID:    records[visible[0]].id,
	}
	return ElementReferenceParticle{facts: facts}, nil
}

func schemaElementReferenceElementCandidates(
	candidates []int,
	records []schemaComponentRecord,
	loc Loc,
) ([]int, error) {
	elementCandidates := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate < 0 || candidate >= len(records) {
			return nil, newSchemaBridgeInvariant(loc, "element reference lookup has an invalid record index")
		}
		if records[candidate].kind != ComponentKindElementDeclaration {
			continue
		}
		elementCandidates = append(elementCandidates, candidate)
	}
	return elementCandidates, nil
}

func schemaElementReferenceVisibleCandidates(
	elementCandidates []int,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	visibleSources map[SourceID][]SourceID,
) ([]int, error) {
	return schemaVisibleCandidates(elementCandidates, owner.id.Source(), records, visibleSources, owner.loc)
}

func schemaElementReferenceVisibilityDiagnostic(
	reference *schemaElementReferenceInput,
	owner schemaComponentRecord,
	records []schemaComponentRecord,
	elementCandidates []int,
	version XSDVersion,
) error {
	related := schemaComponentLocations(records, elementCandidates)
	if reference.name.Namespace() != owner.name.Namespace() {
		return newSchemaElementReferenceImportDiagnostic(
			diagnosticSchemaElementReferenceNamespaceCode,
			reference.loc,
			fmt.Sprintf("element reference %q names a namespace that is not imported into %q", reference.name, owner.name.Namespace()),
			related,
			version,
			errSchemaElementReferenceNamespace,
		)
	}
	return newSchemaElementReferenceDiagnostic(
		diagnosticSchemaElementReferenceUnresolvedCode,
		reference.loc,
		fmt.Sprintf("element reference %q is not visible from its schema document", reference.name),
		related,
		version,
		errSchemaElementReferenceUnresolved,
	)
}

// schemaScalarTypeIsPrecisionDecimal derives the resolved scalar relation
// from the declaration QName and type records; completed particles do not
// duplicate this phase-local fact.
func schemaScalarTypeIsPrecisionDecimal(
	declaredType QName,
	records []schemaComponentRecord,
	byName map[QName][]int,
	referringSource SourceID,
	visibleSources map[SourceID][]SourceID,
	simpleTypes []schemaSimpleTypeResult,
	loc Loc,
	version XSDVersion,
) (bool, error) {
	if declaredType.Namespace() == xsdNamespaceURI {
		return version != XSDVersion10 && declaredType.Local() == "precisionDecimal", nil
	}

	candidates := byName[declaredType]
	if len(candidates) == 0 {
		return false, nil
	}
	visibleCandidates, err := schemaVisibleCandidates(candidates, referringSource, records, visibleSources, loc)
	if err != nil {
		return false, err
	}
	typeCandidate := -1
	for _, candidate := range visibleCandidates {
		kind := records[candidate].kind
		if kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
			continue
		}
		if typeCandidate >= 0 {
			return false, nil
		}
		typeCandidate = candidate
	}
	if typeCandidate < 0 || records[typeCandidate].kind != ComponentKindSimpleTypeDefinition {
		return false, nil
	}
	if typeCandidate >= len(simpleTypes) {
		return false, newSchemaBridgeInvariant(loc, "precisionDecimal type lookup has an incomplete simple type result")
	}
	if !simpleTypes[typeCandidate].present {
		return false, nil
	}
	_, ok := simpleTypes[typeCandidate].facets.(schemaPrecisionDecimalFacetVariant)
	return ok, nil
}

func unsupportedChoicePrecisionDecimalParticle(input *schemaChoiceParticleInput, version XSDVersion) Diagnostic {
	return newSchemaSyntaxUnsupportedForVersion(
		input.loc,
		fmt.Sprintf("non-default choice occurrence range %s with precisionDecimal alternatives is not implemented", input.occurrences),
		version,
	)
}

func unsupportedChoicePrecisionDecimalAlternative(input schemaElementParticleInput, version XSDVersion) Diagnostic {
	loc := input.loc
	if input.typeInput != nil {
		loc = input.typeInput.typeLoc
	}
	return newSchemaSyntaxUnsupportedForVersion(
		loc,
		fmt.Sprintf("non-default choice alternative occurrence range %s for precisionDecimal is not implemented", input.occurrences),
		version,
	)
}

func unresolvedSchemaElementType(input *schemaElementInput, version XSDVersion) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeUnresolvedCode,
		input.typeLoc,
		fmt.Sprintf("element type %q cannot be resolved", input.declaredType),
		nil,
		version,
		errSchemaElementTypeUnresolved,
	)
}

func newSchemaDuplicateDiagnostic(later, earliest schemaComponentRecord, space schemaSymbolSpace, version XSDVersion) Diagnostic {
	if space == schemaSymbolSpaceElement {
		return newSchemaElementDuplicateDiagnostic(later, earliest, version)
	}
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaGlobalDuplicateCode,
		loc:     later.loc,
		message: fmt.Sprintf("global %s %q is duplicated", schemaSymbolSpaceDescription(space), later.name),
		related: []Loc{earliest.loc},
		specRef: schemaGlobalDuplicateSpecRef(version),
		cause:   errSchemaGlobalDeclarationDuplicate,
	}
}

func newSchemaElementDuplicateDiagnostic(later, earliest schemaComponentRecord, version XSDVersion) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaElementDuplicateCode,
		loc:     later.loc,
		message: fmt.Sprintf("global element declaration %q is duplicated", later.name),
		related: []Loc{earliest.loc},
		specRef: schemaElementDuplicateSpecRef(version),
		cause:   errSchemaElementDuplicate,
	}
}

func schemaSymbolSpaceDescription(space schemaSymbolSpace) string {
	switch space {
	case schemaSymbolSpaceElement:
		return "element declaration"
	case schemaSymbolSpaceAttribute:
		return "attribute declaration"
	case schemaSymbolSpaceType:
		return "type definition"
	case schemaSymbolSpaceModelGroup:
		return "model group definition"
	case schemaSymbolSpaceAttributeGroup:
		return "attribute group definition"
	case schemaSymbolSpaceNotation:
		return "notation declaration"
	default:
		return "declaration"
	}
}

func schemaGlobalDuplicateSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaGlobalDuplicateXSD10SpecRef
	}
	return schemaGlobalDuplicateXSD11SpecRef
}

func schemaElementDuplicateSpecRef(version XSDVersion) string {
	return schemaGlobalDuplicateSpecRef(version)
}

func wrongKindSchemaElementType(input *schemaElementInput, related []Loc, version XSDVersion) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeWrongKindCode,
		input.typeLoc,
		fmt.Sprintf("element type %q does not name a simple type", input.declaredType),
		related,
		version,
		fmt.Errorf("%w: %q", errSchemaElementTypeWrongKind, input.declaredType),
	)
}

func ambiguousSchemaElementType(input *schemaElementInput, related []Loc, version XSDVersion) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeAmbiguousCode,
		input.typeLoc,
		fmt.Sprintf("element type %q is ambiguous", input.declaredType),
		related,
		version,
		fmt.Errorf("%w: %q", errSchemaElementTypeAmbiguous, input.declaredType),
	)
}

func newSchemaElementTypeDiagnostic(code string, loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaElementTypeSpecRef(version),
		cause:   cause,
	}
}

func schemaElementTypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementTypeXSD10SpecRef
	}
	return schemaElementTypeXSD11SpecRef
}

func newSchemaSubstitutionGroupCardinalityDiagnostic(loc Loc, message string, version XSDVersion, cause error) Diagnostic {
	return newSchemaSubstitutionGroupDiagnostic(
		invalidSchemaCompositionCode,
		loc,
		message,
		nil,
		cause,
		schemaSubstitutionAffiliationSpecRef(version),
	)
}

func newSchemaSubstitutionGroupQNameDiagnostic(loc Loc, message string, version XSDVersion, cause error) Diagnostic {
	return newSchemaSubstitutionGroupDiagnostic(
		invalidSchemaConditionalCode,
		loc,
		message,
		nil,
		cause,
		schemaSubstitutionQNameSpecRef(version),
	)
}

func newSchemaSubstitutionGroupQNameDiagnosticAtReference(loc Loc, message string, cause error, specRef string) Diagnostic {
	return newSchemaSubstitutionGroupDiagnostic(
		invalidSchemaConditionalCode,
		loc,
		message,
		nil,
		cause,
		specRef,
	)
}

func newSchemaSubstitutionGroupDiagnostic(code string, loc Loc, message string, related []Loc, cause error, specRef string) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: specRef,
		cause:   cause,
	}
}

func newSchemaSubstitutionUnsupported(loc Loc, message string, version XSDVersion, cause error) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			"schema syntax feature is not registered",
			cause,
		)
	}
	return newUnsupportedForVersionWithCause(feature, UnsupportedSchemaSyntaxCode, loc, message, version, cause)
}

func schemaSubstitutionAffiliationSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaSubstitutionAffiliationXSD10SpecRef
	}
	return schemaSubstitutionAffiliationXSD11SpecRef
}

func schemaSubstitutionQNameSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaSubstitutionQNameXSD10SpecRef
	}
	return schemaSubstitutionQNameXSD11SpecRef
}

func schemaSubstitutionResolveSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaSubstitutionResolveXSD10SpecRef
	}
	return schemaSubstitutionResolveXSD11SpecRef
}

func schemaSubstitutionConstraintSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaSubstitutionConstraintXSD10SpecRef
	}
	return schemaSubstitutionConstraintXSD11SpecRef
}

func newSchemaElementReferenceDiagnostic(
	code string,
	loc Loc,
	message string,
	related []Loc,
	version XSDVersion,
	cause error,
) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaElementReferenceSpecRef(version),
		cause:   cause,
	}
}

func newSchemaElementReferenceBlockDiagnostic(attribute syntaxAttribute, version XSDVersion) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaElementReferenceBlockCode,
		loc:     attribute.loc,
		message: `local element ref cannot combine with "block"`,
		specRef: schemaElementReferenceBlockSpecRef(version),
		cause:   errSchemaElementReferenceBlock,
	}
}

func newSchemaElementReferenceDuplicateDiagnostic(reference *schemaElementReferenceInput, firstLoc Loc, version XSDVersion) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaElementReferenceDuplicateCode,
		loc:     reference.loc,
		message: fmt.Sprintf("element reference %q is duplicated in the named model-group choice", reference.name),
		related: []Loc{firstLoc},
		specRef: schemaElementReferenceDuplicateSpecRef(version),
		cause:   fmt.Errorf("%w: %q", errSchemaElementReferenceDuplicate, reference.name),
	}
}

func newSchemaElementReferenceImportDiagnostic(
	code string,
	loc Loc,
	message string,
	related []Loc,
	version XSDVersion,
	cause error,
) Diagnostic {
	diagnostic := newSchemaElementReferenceDiagnostic(code, loc, message, related, version, cause)
	diagnostic.specRef = schemaElementReferenceImportSpecRef(version)
	return diagnostic
}

func schemaElementReferenceSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementReferenceXSD10SpecRef
	}
	return schemaElementReferenceXSD11SpecRef
}

func schemaElementReferenceBlockSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementReferenceBlockXSD10SpecRef
	}
	return schemaElementReferenceBlockXSD11SpecRef
}

func schemaElementReferenceDuplicateSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementReferenceDuplicateXSD10SpecRef
	}
	return schemaElementReferenceDuplicateXSD11SpecRef
}

func schemaElementReferenceImportSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementReferenceImportXSD10SpecRef
	}
	return schemaElementReferenceImportXSD11SpecRef
}

func newSchemaNotationDiagnostic(loc Loc, message string, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaNotationCode,
		loc:     loc,
		message: message,
		specRef: schemaNotationSpecRef(version),
		cause:   cause,
	}
}

func schemaNotationSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaNotationXSD10SpecRef
	}
	return schemaNotationXSD11SpecRef
}

func precisionDecimalSchemaVersionDiagnostic(loc Loc, name QName) Diagnostic {
	return newXSD11FeatureMismatchAtReference(
		FeatureDatatypeFacets,
		diagnosticSchemaPrecisionDecimalVersionCode,
		loc,
		fmt.Sprintf("precisionDecimal type %q is not available under the selected XSD 1.0 policy", name),
		"xsd11-datatypes#dt-primitive",
		fmt.Errorf("%w: %q", errSchemaPrecisionDecimalVersion, name),
	)
}

func (resolver *schemaSimpleTypeResolver) resolve(index int, version XSDVersion) (schemaSimpleTypeResult, error) {
	if index < 0 || index >= len(resolver.records) {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(Loc{}, "simple type resolution index is out of range")
	}
	input := resolver.records[index].simpleType
	if input == nil {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(
			resolver.records[index].loc,
			"simple type resolution has no model input",
		)
	}
	result, err := resolver.resolveInput(input, resolver.records[index].loc, false, resolver.records[index].id.Source(), version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	resolver.results[index] = result
	return result, nil
}

func (resolver *schemaSimpleTypeResolver) resolveInput(input *schemaSimpleTypeInput, fallbackLoc Loc, anonymous bool, source SourceID, version XSDVersion) (schemaSimpleTypeResult, error) {
	if input == nil {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(fallbackLoc, "simple type resolution received a nil model input")
	}
	switch resolver.states[input] {
	case schemaSimpleTypeUnvisited:
	case schemaSimpleTypeResolved:
		return resolver.inputResults[input], nil
	case schemaSimpleTypeVisiting:
		return schemaSimpleTypeResult{}, resolver.cycleDiagnostic(input, version)
	default:
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(fallbackLoc, "simple type resolution has an unknown state")
	}
	model, err := schemaSimpleTypeModel(input)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	resolver.states[input] = schemaSimpleTypeVisiting
	resolver.stack = append(resolver.stack, input)
	resolver.stackFallbackLoc = append(resolver.stackFallbackLoc, fallbackLoc)
	result, err := resolver.resolveModel(input, model, source, version, anonymous)
	if err != nil {
		if popErr := resolver.popInput(input, fallbackLoc); popErr != nil {
			return schemaSimpleTypeResult{}, popErr
		}
		return schemaSimpleTypeResult{}, err
	}
	result.present = true
	result.anonymous = anonymous
	result.loc = input.loc
	if result.loc.IsZero() {
		result.loc = fallbackLoc
	}
	result.nodeID = input.nodeID
	result.hasNodeID = input.hasNodeID
	resolver.states[input] = schemaSimpleTypeResolved
	resolver.inputResults[input] = result
	if err := resolver.popInput(input, fallbackLoc); err != nil {
		return schemaSimpleTypeResult{}, err
	}
	return result, nil
}

func (resolver *schemaSimpleTypeResolver) popInput(input *schemaSimpleTypeInput, fallbackLoc Loc) error {
	last := len(resolver.stack) - 1
	if last < 0 || resolver.stack[last] != input {
		return newSchemaBridgeInvariant(fallbackLoc, "simple type resolution stack pop does not match current input")
	}
	if len(resolver.stackFallbackLoc) != len(resolver.stack) {
		return newSchemaBridgeInvariant(fallbackLoc, "simple type resolution stack locations are inconsistent")
	}
	resolver.stack = resolver.stack[:last]
	resolver.stackFallbackLoc = resolver.stackFallbackLoc[:last]
	return nil
}

func schemaSimpleTypeModel(input *schemaSimpleTypeInput) (schemaSimpleTypeModelInput, error) {
	if input.model != nil {
		return input.model, nil
	}
	if input.base.IsZero() {
		return nil, newSchemaBridgeInvariant(input.loc, "simple type input has no tagged model")
	}
	return &schemaSimpleTypeRestrictionModelInput{
		loc: input.loc,
		base: schemaSimpleTypeReferenceInput{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: input.base,
			loc:  input.baseLoc,
		},
		facets: cloneSchemaFacetInputs(input.facets),
	}, nil
}

func (resolver *schemaSimpleTypeResolver) resolveModel(input *schemaSimpleTypeInput, model schemaSimpleTypeModelInput, source SourceID, version XSDVersion, anonymous bool) (schemaSimpleTypeResult, error) {
	switch typed := model.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if typed == nil {
			return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type restriction model is nil")
		}
		return resolver.resolveRestrictionModel(input, typed, source, version, anonymous)
	case *schemaSimpleTypeListModelInput:
		if typed == nil {
			return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type list model is nil")
		}
		return resolver.resolveListModel(input, typed, source, version)
	case *schemaSimpleTypeUnionModelInput:
		if typed == nil {
			return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type union model is nil")
		}
		return resolver.resolveUnionModel(input, typed, source, version)
	default:
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type has an unknown tagged model")
	}
}

func (resolver *schemaSimpleTypeResolver) resolveRestrictionModel(input *schemaSimpleTypeInput, model *schemaSimpleTypeRestrictionModelInput, source SourceID, version XSDVersion, anonymous bool) (schemaSimpleTypeResult, error) {
	base, err := resolver.resolveReference(model.base, source, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	if base.variety != SimpleTypeVarietyAtomicRestriction {
		return schemaSimpleTypeResult{}, unsupportedSimpleTypeRestriction(
			model.base.loc,
			base,
			version,
		)
	}
	if enumerationErr := rejectAnonymousNonStringEnumeration(base, model.facets, version, anonymous); enumerationErr != nil {
		return schemaSimpleTypeResult{}, enumerationErr
	}
	facets, err := restrictSchemaSimpleTypeFacets(base.facets, model.facets, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	result := schemaSimpleTypeResult{
		loc:              input.loc,
		variety:          SimpleTypeVarietyAtomicRestriction,
		varietyLoc:       model.loc,
		atomicKind:       base.atomicKind,
		baseReference:    base,
		hasBaseReference: true,
		facets:           facets,
		baseLoc:          model.base.loc,
	}
	if base.kind != SimpleTypeReferenceAnonymous {
		result.base = base.name
	}
	if base.hasID {
		result.baseID = base.id
		result.hasBaseID = true
	}
	return result, nil
}

func rejectAnonymousNonStringEnumeration(base schemaSimpleTypeReferenceComponent, inputs []schemaFacetInput, version XSDVersion, anonymous bool) error {
	if !anonymous {
		return nil
	}
	if _, ok := base.facets.(schemaStringFacetVariant); ok {
		return nil
	}
	for _, input := range inputs {
		if input.kind == schemaFacetEnumeration {
			return unsupportedSchemaDatatypeFacet(input, version)
		}
	}
	return nil
}

func (resolver *schemaSimpleTypeResolver) resolveListModel(input *schemaSimpleTypeInput, model *schemaSimpleTypeListModelInput, source SourceID, version XSDVersion) (schemaSimpleTypeResult, error) {
	itemType, err := resolver.resolveReference(model.itemType, source, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	if itemType.variety != SimpleTypeVarietyAtomicRestriction && itemType.variety != SimpleTypeVarietyUnion {
		return schemaSimpleTypeResult{}, invalidSimpleTypeDerivation(
			model.itemType.loc,
			fmt.Sprintf("simple type list item type %q has variety %q", itemType.name, itemType.variety),
			resolver.simpleTypeReferenceRelatedLocations(itemType),
			version,
		)
	}
	if version == XSDVersion11 && itemType.variety == SimpleTypeVarietyUnion {
		if err := resolver.validateAtomicUnionMembers(itemType, version, make(map[schemaSimpleTypeReferenceIdentity]struct{})); err != nil {
			return schemaSimpleTypeResult{}, err
		}
	}
	return schemaSimpleTypeResult{
		loc:         input.loc,
		variety:     SimpleTypeVarietyList,
		varietyLoc:  model.loc,
		itemType:    itemType,
		hasItemType: true,
	}, nil
}

type schemaSimpleTypeReferenceIdentity struct {
	kind        SimpleTypeReferenceKind
	name        QName
	id          ComponentID
	anonymousID SimpleTypeID
}

func simpleTypeReferenceIdentity(reference schemaSimpleTypeReferenceComponent) schemaSimpleTypeReferenceIdentity {
	return schemaSimpleTypeReferenceIdentity{
		kind:        reference.kind,
		name:        reference.name,
		id:          reference.id,
		anonymousID: reference.anonymousID,
	}
}

func (resolver *schemaSimpleTypeResolver) validateAtomicUnionMembers(
	reference schemaSimpleTypeReferenceComponent,
	version XSDVersion,
	seen map[schemaSimpleTypeReferenceIdentity]struct{},
) error {
	identity := simpleTypeReferenceIdentity(reference)
	if _, ok := seen[identity]; ok {
		return nil
	}
	seen[identity] = struct{}{}
	if reference.variety == SimpleTypeVarietyAtomicRestriction {
		return nil
	}
	if reference.variety != SimpleTypeVarietyUnion {
		return invalidSimpleTypeDerivation(
			reference.loc,
			fmt.Sprintf("simple type list item type %q has non-atomic transitive member variety %q", reference.name, reference.variety),
			resolver.simpleTypeReferenceRelatedLocations(reference),
			version,
		)
	}
	memberTypes, err := resolver.simpleTypeUnionMembers(reference, version)
	if err != nil {
		return err
	}
	for _, member := range memberTypes {
		if member.variety == SimpleTypeVarietyAtomicRestriction {
			continue
		}
		if err := resolver.validateAtomicUnionMembers(member, version, seen); err != nil {
			return err
		}
	}
	return nil
}

func (resolver *schemaSimpleTypeResolver) simpleTypeUnionMembers(
	reference schemaSimpleTypeReferenceComponent,
	version XSDVersion,
) ([]schemaSimpleTypeReferenceComponent, error) {
	if reference.kind == SimpleTypeReferenceAnonymous {
		if reference.anonymous == nil {
			return nil, newSchemaBridgeInvariant(reference.loc, "anonymous union reference has no model")
		}
		return reference.anonymous.memberTypes, nil
	}
	if reference.kind != SimpleTypeReferenceNamed {
		return nil, newSchemaBridgeInvariant(reference.loc, "union reference is not named or anonymous")
	}
	if !reference.hasID || reference.id.IsZero() {
		return nil, newSchemaBridgeInvariant(reference.loc, "named union reference has no component identity")
	}
	for index, record := range resolver.records {
		if record.id != reference.id {
			continue
		}
		result, err := resolver.resolve(index, version)
		if err != nil {
			return nil, err
		}
		return result.memberTypes, nil
	}
	return nil, newSchemaBridgeInvariant(reference.loc, "named union reference component identity is unresolved")
}

func (resolver *schemaSimpleTypeResolver) simpleTypeReferenceDefinitionLoc(reference schemaSimpleTypeReferenceComponent) Loc {
	if reference.kind == SimpleTypeReferenceAnonymous {
		if reference.anonymous != nil {
			return reference.anonymous.loc
		}
		return Loc{}
	}
	if reference.kind != SimpleTypeReferenceNamed || !reference.hasID {
		return Loc{}
	}
	for _, record := range resolver.records {
		if record.id == reference.id {
			return record.loc
		}
	}
	return Loc{}
}

func (resolver *schemaSimpleTypeResolver) simpleTypeReferenceRelatedLocations(reference schemaSimpleTypeReferenceComponent) []Loc {
	related := make([]Loc, 0, 2)
	definitionLoc := resolver.simpleTypeReferenceDefinitionLoc(reference)
	if !definitionLoc.IsZero() && definitionLoc != reference.loc {
		related = append(related, definitionLoc)
	}
	if !reference.varietyLoc.IsZero() && reference.varietyLoc != reference.loc && reference.varietyLoc != definitionLoc {
		related = append(related, reference.varietyLoc)
	}
	return related
}

func unsupportedSimpleTypeRestriction(loc Loc, base schemaSimpleTypeReferenceComponent, version XSDVersion) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			"schema syntax feature is not registered",
			fmt.Errorf("%w: %s", errSchemaSimpleTypeRestrictionUnsupported, base.variety),
		)
	}
	message := fmt.Sprintf("simple type restriction base %q has variety %q, which is not implemented", base.name, base.variety)
	if base.name.IsZero() {
		message = fmt.Sprintf("simple type restriction base has variety %q, which is not implemented", base.variety)
	}
	diagnostic := newUnsupportedForVersionWithCause(
		feature,
		UnsupportedSchemaSyntaxCode,
		loc,
		message,
		version,
		fmt.Errorf("%w: %s", errSchemaSimpleTypeRestrictionUnsupported, message),
	)
	if diagnostic.Class() == FailureUnsupported {
		diagnostic.specRef = schemaSimpleTypeSpecRef(version)
		if !base.varietyLoc.IsZero() && base.varietyLoc != loc {
			diagnostic.related = []Loc{base.varietyLoc}
		}
	}
	return diagnostic
}

func (resolver *schemaSimpleTypeResolver) resolveUnionModel(input *schemaSimpleTypeInput, model *schemaSimpleTypeUnionModelInput, source SourceID, version XSDVersion) (schemaSimpleTypeResult, error) {
	if len(model.members) == 0 {
		return schemaSimpleTypeResult{}, newSchemaCompositionDiagnostic(model.loc, "simple type union requires at least one member type")
	}
	members := make([]schemaSimpleTypeReferenceComponent, 0, len(model.members))
	for _, member := range model.members {
		resolved, err := resolver.resolveReference(member, source, version)
		if err != nil {
			return schemaSimpleTypeResult{}, err
		}
		if version == XSDVersion10 && resolved.variety == SimpleTypeVarietyUnion {
			return schemaSimpleTypeResult{}, invalidSimpleTypeDerivation(
				member.loc,
				fmt.Sprintf("simple type union member type %q has variety %q", resolved.name, resolved.variety),
				resolver.simpleTypeReferenceRelatedLocations(resolved),
				version,
			)
		}
		members = append(members, resolved)
	}
	return schemaSimpleTypeResult{
		loc:         input.loc,
		variety:     SimpleTypeVarietyUnion,
		varietyLoc:  model.loc,
		memberTypes: members,
	}, nil
}

func (resolver *schemaSimpleTypeResolver) resolveReference(input schemaSimpleTypeReferenceInput, source SourceID, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	switch input.kind {
	case schemaSimpleTypeAnonymousReferenceInput:
		if input.anonymous == nil {
			return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "anonymous simple type reference has no model")
		}
		result, err := resolver.resolveInput(input.anonymous, input.loc, true, source, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		if !result.hasNodeID || result.nodeID.IsZero() {
			return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "anonymous simple type reference has no allocated model identity")
		}
		return schemaSimpleTypeReferenceComponent{
			kind:           SimpleTypeReferenceAnonymous,
			loc:            input.loc,
			anonymousID:    result.nodeID,
			hasAnonymousID: result.hasNodeID,
			anonymous:      schemaSimpleTypeComponentFromResult(result, true),
			variety:        result.variety,
			varietyLoc:     result.varietyLoc,
			atomicKind:     result.atomicKind,
			facets:         result.facets,
		}, nil
	case schemaSimpleTypeQNameReferenceInput:
		if input.name.IsZero() {
			return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "simple type QName reference is empty")
		}
		if input.name.Namespace() == xsdNamespaceURI {
			return resolveBuiltinSchemaSimpleTypeReference(input, version)
		}
		return resolver.resolveNamedSchemaSimpleTypeReference(input, source, version)
	default:
		return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "simple type reference has an unknown kind")
	}
}

//nolint:gocognit,funlen // Keep the built-in datatype mapping explicit and versioned.
func resolveBuiltinSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	result := schemaSimpleTypeResult{
		variety:    SimpleTypeVarietyAtomicRestriction,
		varietyLoc: input.loc,
	}
	switch input.name.Local() {
	case "string":
		return resolveBuiltinStringSchemaSimpleTypeReference(input, version)
	case "token":
		return resolveBuiltinTokenSchemaSimpleTypeReference(input, version)
	case "integer":
		facets, err := NewIntegerDigitFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		bounds, err := NewIntegerBoundFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicInteger
		result.facets = schemaDigitFacetVariant{value: facets, integerBounds: bounds}
	case "negativeInteger":
		facets, err := NewIntegerDigitFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		maxInclusive, err := ParseIntegerMaxInclusiveFacet("-1", input.loc, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{maxInclusive}, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicNegativeInteger
		result.facets = schemaDigitFacetVariant{value: facets, integerBounds: bounds}
	case "decimal":
		facets, err := NewDecimalDigitFacets(nil, nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		bounds, err := NewDecimalBoundFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicDecimal
		result.facets = schemaDigitFacetVariant{value: facets, decimalBounds: bounds}
	case "precisionDecimal":
		if version == XSDVersion10 {
			return schemaSimpleTypeReferenceComponent{}, precisionDecimalSchemaVersionDiagnostic(input.loc, input.name)
		}
		facets, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{})
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicPrecisionDecimal
		result.facets = schemaPrecisionDecimalFacetVariant{value: facets}
	case "boolean":
		result.facets = schemaBooleanFacetVariant{}
	case "language":
		result.atomicKind = schemaSimpleTypeAtomicLanguage
		result.facets = schemaAtomicFacetVariant{}
	case "NCName":
		result.atomicKind = schemaSimpleTypeAtomicNCName
		result.facets = schemaAtomicFacetVariant{}
	case "anyURI":
		result.atomicKind = schemaSimpleTypeAtomicAnyURI
		result.facets = schemaAtomicFacetVariant{}
	case "ID":
		result.atomicKind = schemaSimpleTypeAtomicID
		result.facets = schemaAtomicFacetVariant{}
	default:
		return schemaSimpleTypeReferenceComponent{}, newSchemaSyntaxUnsupported(
			input.loc,
			fmt.Sprintf("simple type reference %q is not supported", input.name),
		)
	}
	return schemaSimpleTypeReferenceComponent{
		kind:       SimpleTypeReferenceBuiltin,
		name:       input.name,
		loc:        input.loc,
		variety:    result.variety,
		varietyLoc: result.varietyLoc,
		atomicKind: result.atomicKind,
		facets:     result.facets,
	}, nil
}

func resolveBuiltinStringSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	return resolveBuiltinStringLikeSchemaSimpleTypeReference(input, version, schemaSimpleTypeAtomicString, defaultStringWhiteSpaceFacet())
}

func resolveBuiltinTokenSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	return resolveBuiltinStringLikeSchemaSimpleTypeReference(input, version, schemaSimpleTypeAtomicToken, defaultTokenWhiteSpaceFacet())
}

func resolveBuiltinStringLikeSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, version XSDVersion, atomicKind schemaSimpleTypeAtomicKind, whiteSpace *StringWhiteSpaceFacet) (schemaSimpleTypeReferenceComponent, error) {
	enumeration, err := NewStringEnumerationFacets(nil, version)
	if err != nil {
		return schemaSimpleTypeReferenceComponent{}, err
	}
	return schemaSimpleTypeReferenceComponent{
		kind:       SimpleTypeReferenceBuiltin,
		name:       input.name,
		loc:        input.loc,
		variety:    SimpleTypeVarietyAtomicRestriction,
		varietyLoc: input.loc,
		atomicKind: atomicKind,
		facets:     schemaStringFacetVariant{enumeration: enumeration, whiteSpace: whiteSpace},
	}, nil
}

func (resolver *schemaSimpleTypeResolver) resolveNamedSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, source SourceID, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	candidates := resolver.byName[input.name]
	if len(candidates) == 0 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeUnresolvedCode,
			input.loc,
			fmt.Sprintf("simple type reference %q cannot be resolved", input.name),
			nil,
			version,
			errSchemaSimpleTypeBaseUnresolved,
		)
	}
	visibleCandidates, err := schemaVisibleCandidates(candidates, source, resolver.records, resolver.visibleSources, input.loc)
	if err != nil {
		return schemaSimpleTypeReferenceComponent{}, err
	}
	if len(visibleCandidates) == 0 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeUnresolvedCode,
			input.loc,
			fmt.Sprintf("simple type reference %q cannot be resolved", input.name),
			nil,
			version,
			errSchemaSimpleTypeBaseUnresolved,
		)
	}
	simpleCandidates := make([]int, 0, len(visibleCandidates))
	for _, candidate := range visibleCandidates {
		if resolver.records[candidate].kind != ComponentKindSimpleTypeDefinition {
			continue
		}
		simpleCandidates = append(simpleCandidates, candidate)
	}
	if len(simpleCandidates) == 0 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeWrongKindCode,
			input.loc,
			fmt.Sprintf("simple type reference %q does not name a simple type", input.name),
			schemaComponentLocations(resolver.records, visibleCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseWrongKind, input.name),
		)
	}
	if len(simpleCandidates) > 1 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeAmbiguousCode,
			input.loc,
			fmt.Sprintf("simple type reference %q is ambiguous", input.name),
			schemaComponentLocations(resolver.records, simpleCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseAmbiguous, input.name),
		)
	}
	index := simpleCandidates[0]
	result, err := resolver.resolve(index, version)
	if err != nil {
		return schemaSimpleTypeReferenceComponent{}, err
	}
	return schemaSimpleTypeReferenceComponent{
		kind:       SimpleTypeReferenceNamed,
		name:       input.name,
		loc:        input.loc,
		id:         resolver.records[index].id,
		hasID:      true,
		variety:    result.variety,
		varietyLoc: result.varietyLoc,
		atomicKind: result.atomicKind,
		facets:     result.facets,
	}, nil
}

func schemaSimpleTypeComponentFromResult(result schemaSimpleTypeResult, anonymous bool) *schemaSimpleTypeComponent {
	return &schemaSimpleTypeComponent{
		loc:              result.loc,
		nodeID:           result.nodeID,
		hasNodeID:        result.hasNodeID,
		anonymous:        anonymous,
		variety:          result.variety,
		varietyLoc:       result.varietyLoc,
		atomicKind:       result.atomicKind,
		base:             result.base,
		baseLoc:          result.baseLoc,
		baseID:           result.baseID,
		hasBaseID:        result.hasBaseID,
		baseReference:    result.baseReference,
		hasBaseReference: result.hasBaseReference,
		itemType:         result.itemType,
		hasItemType:      result.hasItemType,
		memberTypes:      cloneSchemaSimpleTypeReferenceComponents(result.memberTypes),
		facets:           result.facets,
	}
}

func invalidSimpleTypeDerivation(loc Loc, message string, related []Loc, version XSDVersion) Diagnostic {
	filteredRelated := make([]Loc, 0, len(related))
	for _, relatedLoc := range related {
		if relatedLoc.IsZero() || relatedLoc == loc {
			continue
		}
		filteredRelated = append(filteredRelated, relatedLoc)
	}
	return newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeBaseCode,
		loc,
		message,
		filteredRelated,
		version,
		fmt.Errorf("%w: %s", errSchemaSimpleTypeInvalidDerivation, message),
	)
}

//nolint:gocognit // Keep numeric and precision facet construction in one phase.
func restrictSchemaSimpleTypeFacets(
	base schemaSimpleTypeFacetVariant,
	inputs []schemaFacetInput,
	version XSDVersion,
) (schemaSimpleTypeFacetVariant, error) {
	switch typed := base.(type) {
	case schemaDigitFacetVariant:
		switch typed.value.Kind() {
		case DigitDatatypeInteger:
			baseEnumeration, err := NewIntegerEnumerationFacets(nil, version)
			if err != nil {
				return nil, err
			}
			baseBounds, err := NewIntegerBoundFacets(nil, version)
			if err != nil {
				return nil, err
			}
			if typed.integerBounds.Version() == version {
				baseBounds = typed.integerBounds
			}
			return restrictSchemaIntegerFacets(typed.value, baseBounds, baseEnumeration, inputs, version)
		case DigitDatatypeDecimal:
			baseEnumeration, err := NewDecimalEnumerationFacets(nil, version)
			if err != nil {
				return nil, err
			}
			baseBounds, err := NewDecimalBoundFacets(nil, version)
			if err != nil {
				return nil, err
			}
			if typed.decimalBounds.Version() == version {
				baseBounds = typed.decimalBounds
			}
			return restrictSchemaDecimalFacets(typed.value, baseBounds, baseEnumeration, inputs, version)
		default:
			return nil, newSchemaBridgeInvariant(Loc{}, "simple type facet resolution has an unknown digit datatype")
		}
	case schemaIntegerFacetVariant:
		return restrictSchemaIntegerFacets(typed.digits, typed.bounds, typed.enumeration, inputs, version)
	case schemaDecimalFacetVariant:
		return restrictSchemaDecimalFacets(typed.digits, typed.bounds, typed.enumeration, inputs, version)
	case schemaPrecisionDecimalFacetVariant:
		local, err := schemaPrecisionDecimalFacetDeclarations(inputs)
		if err != nil {
			return nil, err
		}
		facets, err := RestrictPrecisionDecimalFacets(typed.value, local)
		if err != nil {
			return nil, err
		}
		return schemaPrecisionDecimalFacetVariant{value: facets}, nil
	case schemaStringFacetVariant:
		return restrictSchemaStringFacets(typed, inputs, version)
	case schemaBooleanFacetVariant:
		return restrictSchemaBooleanFacets(typed, inputs, version)
	case schemaAtomicFacetVariant:
		if len(inputs) == 0 {
			return typed, nil
		}
		if err := validateStringWhiteSpaceFacetInputs(inputs, version); err != nil {
			return nil, err
		}
		return nil, unsupportedSchemaDatatypeFacet(inputs[0], version)
	default:
		return nil, newSchemaBridgeInvariant(Loc{}, "simple type facet resolution has an unknown datatype variant")
	}
}

func restrictSchemaStringFacets(base schemaStringFacetVariant, inputs []schemaFacetInput, version XSDVersion) (schemaSimpleTypeFacetVariant, error) {
	local, err := schemaStringFacetDeclarations(inputs, version)
	if err != nil {
		return nil, err
	}
	enumeration, err := restrictSchemaStringEnumeration(base, local.enumeration)
	if err != nil {
		return nil, err
	}
	whiteSpace, err := restrictStringWhiteSpaceFacet(base.whiteSpace, local.whiteSpace, version)
	if err != nil {
		return nil, err
	}
	if local.deferredUnsupported != nil {
		return nil, local.deferredUnsupported
	}
	return schemaStringFacetVariant{enumeration: enumeration, whiteSpace: &whiteSpace}, nil
}

func restrictSchemaStringEnumeration(base schemaStringFacetVariant, local StringEnumerationFacetDeclarations) (StringEnumerationFacets, error) {
	if base.whiteSpace != nil && base.whiteSpace.Value() == "collapse" {
		return restrictStringEnumerationFacetsInValueSpace(base.enumeration, local)
	}
	return RestrictStringEnumerationFacets(base.enumeration, local)
}

func restrictSchemaIntegerFacets(
	base DigitFacets,
	baseBounds IntegerBoundFacets,
	baseEnumeration IntegerEnumerationFacets,
	inputs []schemaFacetInput,
	version XSDVersion,
) (schemaSimpleTypeFacetVariant, error) {
	local, err := schemaNumericFacetDeclarations(inputs, DigitDatatypeInteger, version)
	if err != nil {
		return nil, err
	}
	digits, err := RestrictDigitFacets(base, local.digits)
	if err != nil {
		return nil, err
	}
	bounds, err := RestrictIntegerBoundFacets(baseBounds, local.integerBounds)
	if err != nil {
		return nil, err
	}
	err = validateSchemaIntegerEnumerationBaseValueSpace(base, baseBounds, local.integerEnumeration, version)
	if err != nil {
		return nil, err
	}
	enumeration, err := RestrictIntegerEnumerationFacets(baseEnumeration, local.integerEnumeration)
	if err != nil {
		return nil, err
	}
	if local.deferredUnsupported != nil {
		return nil, local.deferredUnsupported
	}
	return schemaIntegerFacetVariant{digits: digits, enumeration: enumeration, bounds: bounds}, nil
}

func restrictSchemaDecimalFacets(
	base DigitFacets,
	baseBounds DecimalBoundFacets,
	baseEnumeration DecimalEnumerationFacets,
	inputs []schemaFacetInput,
	version XSDVersion,
) (schemaSimpleTypeFacetVariant, error) {
	local, err := schemaNumericFacetDeclarations(inputs, DigitDatatypeDecimal, version)
	if err != nil {
		return nil, err
	}
	digits, err := RestrictDigitFacets(base, local.digits)
	if err != nil {
		return nil, err
	}
	bounds, err := RestrictDecimalBoundFacets(baseBounds, local.decimalBounds)
	if err != nil {
		return nil, err
	}
	err = validateSchemaDecimalEnumerationBaseValueSpace(base, baseBounds, local.decimalEnumeration, version)
	if err != nil {
		return nil, err
	}
	enumeration, err := RestrictDecimalEnumerationFacets(baseEnumeration, local.decimalEnumeration)
	if err != nil {
		return nil, err
	}
	if local.deferredUnsupported != nil {
		return nil, local.deferredUnsupported
	}
	return schemaDecimalFacetVariant{digits: digits, enumeration: enumeration, bounds: bounds}, nil
}

func validateSchemaIntegerEnumerationBaseValueSpace(
	base DigitFacets,
	bounds IntegerBoundFacets,
	local IntegerEnumerationFacetDeclarations,
	version XSDVersion,
) error {
	if local.Values == nil {
		return nil
	}
	for index := range local.Values {
		declaration := local.Values[index]
		err := base.ValidateInteger(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errDigitFacetValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaDigitFacetViolationLocations(base, err),
				version,
				"integer",
				err,
			)
		}
		err = bounds.ValidateInteger(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errBoundValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaFacetViolationLocations(err),
				version,
				"integer",
				err,
			)
		}
	}
	return nil
}

func validateSchemaDecimalEnumerationBaseValueSpace(
	base DigitFacets,
	bounds DecimalBoundFacets,
	local DecimalEnumerationFacetDeclarations,
	version XSDVersion,
) error {
	if local.Values == nil {
		return nil
	}
	for index := range local.Values {
		declaration := local.Values[index]
		err := base.ValidateDecimal(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errDigitFacetValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaDigitFacetViolationLocations(base, err),
				version,
				"decimal",
				err,
			)
		}
		err = bounds.ValidateDecimal(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errBoundValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaFacetViolationLocations(err),
				version,
				"decimal",
				err,
			)
		}
	}
	return nil
}

func schemaFacetViolationLocations(violation error) []Loc {
	var diagnostic Diagnostic
	if errors.As(violation, &diagnostic) {
		return diagnostic.Related()
	}
	return nil
}

func schemaDigitFacetViolationLocations(base DigitFacets, violation error) []Loc {
	var diagnostic Diagnostic
	if errors.As(violation, &diagnostic) {
		if related := diagnostic.Related(); len(related) != 0 {
			return related
		}
	}
	related := make([]Loc, 0, 2)
	if loc, ok := base.TotalDigitsLoc(); ok && !loc.IsZero() {
		related = append(related, loc)
	}
	if loc, ok := base.FractionDigitsLoc(); ok && !loc.IsZero() {
		related = append(related, loc)
	}
	return related
}

func schemaEnumerationBaseValueSpaceDiagnostic(
	loc Loc,
	related []Loc,
	version XSDVersion,
	datatype string,
	cause error,
) error {
	return newEnumerationDiagnostic(
		FailureInvalid,
		InvalidEnumerationRestrictionCode,
		loc,
		enumerationSpecRef(version, enumerationRestrictionRule),
		"local "+datatype+" enumeration value is not in the base value space",
		related,
		fmt.Errorf("%w: %w", errInvalidEnumerationRestriction, cause),
	)
}

type schemaStringFacetDeclarationSet struct {
	enumeration         StringEnumerationFacetDeclarations
	whiteSpace          *StringWhiteSpaceFacet
	deferredUnsupported error
}

func schemaStringFacetDeclarations(inputs []schemaFacetInput, version XSDVersion) (schemaStringFacetDeclarationSet, error) {
	var enumeration []StringEnumerationFacet
	var whiteSpace *StringWhiteSpaceFacet
	var deferredUnsupported error
	for _, input := range inputs {
		declaration, err := schemaStringFacetDeclaration(input, whiteSpace != nil, version)
		if err != nil {
			return schemaStringFacetDeclarationSet{}, err
		}
		if declaration.enumeration != nil {
			enumeration = append(enumeration, *declaration.enumeration)
			continue
		}
		if declaration.whiteSpace != nil {
			whiteSpace = declaration.whiteSpace
			continue
		}
		if declaration.deferredUnsupported == nil {
			continue
		}
		if deferredUnsupported == nil {
			deferredUnsupported = declaration.deferredUnsupported
		}
	}
	return schemaStringFacetDeclarationSet{
		enumeration:         NewStringEnumerationFacetDeclarations(enumeration),
		whiteSpace:          whiteSpace,
		deferredUnsupported: deferredUnsupported,
	}, nil
}

type schemaStringFacetDeclarationResult struct {
	enumeration         *StringEnumerationFacet
	whiteSpace          *StringWhiteSpaceFacet
	deferredUnsupported error
}

func schemaStringFacetDeclaration(input schemaFacetInput, duplicateWhiteSpace bool, version XSDVersion) (schemaStringFacetDeclarationResult, error) {
	if input.kind == schemaFacetEnumeration {
		facet, err := ParseStringEnumerationFacetFor(version, input.lexical, schemaFacetValueLocation(input))
		if err != nil {
			return schemaStringFacetDeclarationResult{}, err
		}
		return schemaStringFacetDeclarationResult{enumeration: &facet}, nil
	}
	if input.kind == schemaFacetWhiteSpace {
		facet, err := schemaStringWhiteSpaceDeclaration(input, duplicateWhiteSpace, version)
		if err != nil {
			return schemaStringFacetDeclarationResult{}, err
		}
		return schemaStringFacetDeclarationResult{whiteSpace: facet}, nil
	}
	deferred, err := schemaStringUnsupportedFacet(input, version)
	if err != nil {
		return schemaStringFacetDeclarationResult{}, err
	}
	return schemaStringFacetDeclarationResult{deferredUnsupported: deferred}, nil
}

func schemaStringUnsupportedFacet(input schemaFacetInput, version XSDVersion) (error, error) {
	err := unsupportedSchemaDatatypeFacet(input, version)
	if err == nil {
		return nil, nil
	}
	if isUnsupportedSchemaDatatypeFacetError(err) {
		return err, nil
	}
	return nil, err
}

func schemaStringWhiteSpaceDeclaration(input schemaFacetInput, duplicate bool, version XSDVersion) (*StringWhiteSpaceFacet, error) {
	if duplicate {
		return nil, duplicateStringWhiteSpaceFacetDiagnostic(schemaFacetValueLocation(input), version)
	}
	facet, err := ParseStringWhiteSpaceFacetForWithFixed(version, input.lexical, schemaFacetValueLocation(input), input.fixed)
	if err != nil {
		return nil, err
	}
	return &facet, nil
}

func isUnsupportedSchemaDatatypeFacetError(err error) bool {
	var diagnostic Diagnostic
	return errors.As(err, &diagnostic) && diagnostic.Class() == FailureUnsupported
}

type schemaNumericFacetDeclarationSet struct {
	digits              DigitFacetDeclarations
	integerEnumeration  IntegerEnumerationFacetDeclarations
	decimalEnumeration  DecimalEnumerationFacetDeclarations
	integerBounds       IntegerBoundFacetDeclarations
	decimalBounds       DecimalBoundFacetDeclarations
	deferredUnsupported error
}

func restrictSchemaBooleanFacets(base schemaBooleanFacetVariant, inputs []schemaFacetInput, version XSDVersion) (schemaSimpleTypeFacetVariant, error) {
	if len(inputs) == 0 {
		return base, nil
	}

	var unsupported error
	for _, input := range inputs {
		err := unsupportedSchemaBooleanFacet(input, version)
		if err == nil {
			return nil, newSchemaBridgeInvariant(input.loc, "boolean facet restriction produced no result")
		}
		var diagnostic Diagnostic
		if !errors.As(err, &diagnostic) {
			return nil, err
		}
		if diagnostic.Class() != FailureUnsupported {
			return nil, err
		}
		if unsupported == nil {
			unsupported = err
		}
	}
	return nil, unsupported
}

func unsupportedSchemaBooleanFacet(input schemaFacetInput, version XSDVersion) error {
	if err := validateSchemaBooleanFacetInput(input, version); err != nil {
		return err
	}
	err := unsupportedSchemaDatatypeFacet(input, version)
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
		return err
	}
	diagnostic.specRef = schemaBooleanDatatypeSpecRef(version)
	return diagnostic
}

func validateSchemaBooleanFacetInput(input schemaFacetInput, version XSDVersion) error {
	if err := validateOrdinarySchemaFacetInput(input); err != nil {
		return err
	}
	valueLoc := schemaFacetValueLocation(input)
	switch input.kind {
	case schemaFacetEnumeration, schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive:
		_, err := ParseStrictBooleanFor(version, input.lexical, valueLoc)
		return err
	case schemaFacetTotalDigits:
		_, err := ParseTotalDigitsFor(version, input.lexical, valueLoc)
		return err
	case schemaFacetFractionDigits:
		_, err := ParseFractionDigitsFor(version, input.lexical, valueLoc)
		return err
	case schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength:
		value, err := ParseStrictInteger(input.lexical, valueLoc)
		if err != nil {
			return err
		}
		if value.Sign() < 0 {
			return newSchemaCompositionDiagnostic(valueLoc, schemaFacetName(input.kind)+" facet value must be non-negative")
		}
		return nil
	case schemaFacetWhiteSpace:
		if _, err := ParseStringWhiteSpaceFacetFor(version, input.lexical, valueLoc); err != nil {
			return err
		}
		return validateSchemaBooleanFacetEnum(input, "collapse")
	case schemaFacetExplicitTimezone:
		return validateSchemaBooleanFacetEnum(input, "prohibited", "optional", "required")
	case schemaFacetPattern:
		return nil
	case schemaFacetMinScale, schemaFacetMaxScale, schemaFacetPrecision:
		return nil
	default:
		return newSchemaBridgeInvariant(input.loc, "boolean facet has an unknown kind")
	}
}

func validateSchemaBooleanFacetEnum(input schemaFacetInput, values ...string) error {
	value := collapseXMLWhitespace(input.lexical)
	for _, allowed := range values {
		if value == allowed {
			return nil
		}
	}
	return newSchemaCompositionDiagnostic(
		schemaFacetValueLocation(input),
		schemaFacetName(input.kind)+" facet value is invalid",
	)
}

func schemaBooleanDatatypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaBooleanDatatypeXSD10SpecRef
	}
	return schemaBooleanDatatypeXSD11SpecRef
}

//nolint:gocognit,funlen // Keep the supported numeric facet mapping and unsupported facet boundary in one lexical pass.
func schemaNumericFacetDeclarations(
	inputs []schemaFacetInput,
	kind DigitDatatype,
	version XSDVersion,
) (schemaNumericFacetDeclarationSet, error) {
	var totalDigits *TotalDigitsFacet
	var fractionDigits *FractionDigitsFacet
	var integerEnumeration []IntegerEnumerationFacet
	var decimalEnumeration []DecimalEnumerationFacet
	var integerBounds []IntegerBoundFacet
	var decimalBounds []DecimalBoundFacet
	var deferredUnsupported error
	if kind != DigitDatatypeInteger && kind != DigitDatatypeDecimal {
		return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(Loc{}, "simple type facet collection has an unknown digit datatype")
	}
	for _, input := range inputs {
		loc := schemaFacetValueLocation(input)
		switch input.kind {
		case schemaFacetTotalDigits:
			facet, err := ParseTotalDigitsFacetWithFixed(input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			totalDigits = &facet
		case schemaFacetFractionDigits:
			facet, err := ParseFractionDigitsFacetWithFixed(input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			fractionDigits = &facet
		case schemaFacetEnumeration:
			enumerationLoc := input.loc
			switch kind {
			case DigitDatatypeInteger:
				facet, err := ParseIntegerEnumerationFacetFor(version, input.lexical, enumerationLoc)
				if err != nil {
					return schemaNumericFacetDeclarationSet{}, err
				}
				integerEnumeration = append(integerEnumeration, facet)
			case DigitDatatypeDecimal:
				facet, err := ParseDecimalEnumerationFacetFor(version, input.lexical, enumerationLoc)
				if err != nil {
					return schemaNumericFacetDeclarationSet{}, err
				}
				decimalEnumeration = append(decimalEnumeration, facet)
			default:
				return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "numeric facet declarations have an unknown datatype")
			}
		case schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive:
			boundKind, ok := schemaBoundKindFromFacet(input.kind)
			if !ok {
				return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "simple type bound facet has an unknown kind")
			}
			if kind == DigitDatatypeInteger {
				facet, err := ParseIntegerBoundFacetWithFixed(boundKind, input.lexical, loc, input.fixed, version)
				if err != nil {
					return schemaNumericFacetDeclarationSet{}, err
				}
				integerBounds = append(integerBounds, facet)
				continue
			}
			facet, err := ParseDecimalBoundFacetWithFixed(boundKind, input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			decimalBounds = append(decimalBounds, facet)
		case schemaFacetWhiteSpace:
			if _, err := ParseStringWhiteSpaceFacetFor(version, input.lexical, loc); err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			err := unsupportedSchemaDatatypeFacet(input, version)
			if err == nil {
				continue
			}
			var diagnostic Diagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
				return schemaNumericFacetDeclarationSet{}, err
			}
			if deferredUnsupported == nil {
				deferredUnsupported = err
			}
		case schemaFacetMinScale, schemaFacetMaxScale, schemaFacetPattern,
			schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength,
			schemaFacetPrecision, schemaFacetExplicitTimezone:
			err := unsupportedSchemaDatatypeFacet(input, version)
			if err == nil {
				continue
			}
			var diagnostic Diagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
				return schemaNumericFacetDeclarationSet{}, err
			}
			if deferredUnsupported == nil {
				deferredUnsupported = err
			}
		default:
			return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "simple type facet has an unknown kind")
		}
	}
	return schemaNumericFacetDeclarationSet{
		digits:              NewDigitFacetDeclarations(totalDigits, fractionDigits),
		integerEnumeration:  NewIntegerEnumerationFacetDeclarations(integerEnumeration),
		decimalEnumeration:  NewDecimalEnumerationFacetDeclarations(decimalEnumeration),
		integerBounds:       NewIntegerBoundFacetDeclarations(integerBounds),
		decimalBounds:       NewDecimalBoundFacetDeclarations(decimalBounds),
		deferredUnsupported: deferredUnsupported,
	}, nil
}

func schemaBoundKindFromFacet(kind schemaFacetKind) (BoundKind, bool) {
	switch kind {
	case schemaFacetMinInclusive:
		return BoundMinInclusive, true
	case schemaFacetMinExclusive:
		return BoundMinExclusive, true
	case schemaFacetMaxInclusive:
		return BoundMaxInclusive, true
	case schemaFacetMaxExclusive:
		return BoundMaxExclusive, true
	case schemaFacetTotalDigits, schemaFacetFractionDigits, schemaFacetMinScale, schemaFacetMaxScale,
		schemaFacetPattern, schemaFacetEnumeration, schemaFacetWhiteSpace, schemaFacetLength,
		schemaFacetMinLength, schemaFacetMaxLength, schemaFacetPrecision, schemaFacetExplicitTimezone:
		return 0, false
	default:
		return 0, false
	}
}

//nolint:funlen // Keep the facet-kind to parser mapping explicit and located.
func schemaPrecisionDecimalFacetDeclarations(inputs []schemaFacetInput) (PrecisionDecimalFacetDeclarations, error) {
	var totalDigits *PrecisionDecimalTotalDigitsFacet
	var minScale *PrecisionDecimalMinScaleFacet
	var maxScale *PrecisionDecimalMaxScaleFacet
	var whiteSpace *PrecisionDecimalWhiteSpaceFacet
	var patterns []PrecisionDecimalPatternFacet
	var enumeration []PrecisionDecimalEnumerationFacet
	var minInclusive *PrecisionDecimalMinInclusiveFacet
	var minExclusive *PrecisionDecimalMinExclusiveFacet
	var maxInclusive *PrecisionDecimalMaxInclusiveFacet
	var maxExclusive *PrecisionDecimalMaxExclusiveFacet
	for _, input := range inputs {
		loc := schemaFacetValueLocation(input)
		var err error
		switch input.kind {
		case schemaFacetTotalDigits:
			facet, parseErr := ParsePrecisionDecimalTotalDigitsFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			totalDigits = &facet
		case schemaFacetMinScale:
			facet, parseErr := ParsePrecisionDecimalMinScaleFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			minScale = &facet
		case schemaFacetMaxScale:
			facet, parseErr := ParsePrecisionDecimalMaxScaleFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			maxScale = &facet
		case schemaFacetPattern:
			facet, parseErr := ParsePrecisionDecimalPatternFacet(input.lexical, loc)
			err = parseErr
			patterns = append(patterns, facet)
		case schemaFacetEnumeration:
			facet, parseErr := ParsePrecisionDecimalEnumerationFacet(input.lexical, loc)
			err = parseErr
			enumeration = append(enumeration, facet)
		case schemaFacetMinInclusive:
			facet, parseErr := ParsePrecisionDecimalMinInclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			minInclusive = &facet
		case schemaFacetMinExclusive:
			facet, parseErr := ParsePrecisionDecimalMinExclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			minExclusive = &facet
		case schemaFacetMaxInclusive:
			facet, parseErr := ParsePrecisionDecimalMaxInclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			maxInclusive = &facet
		case schemaFacetMaxExclusive:
			facet, parseErr := ParsePrecisionDecimalMaxExclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			maxExclusive = &facet
		case schemaFacetWhiteSpace:
			facet, parseErr := ParsePrecisionDecimalWhiteSpaceFacet(input.lexical, loc)
			facet.fixed = input.fixed
			err = parseErr
			whiteSpace = &facet
		case schemaFacetFractionDigits, schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength, schemaFacetPrecision, schemaFacetExplicitTimezone:
			return PrecisionDecimalFacetDeclarations{}, ValidatePrecisionDecimalFacetName(schemaFacetName(input.kind), loc)
		default:
			return PrecisionDecimalFacetDeclarations{}, ValidatePrecisionDecimalFacetName(schemaFacetName(input.kind), loc)
		}
		if err != nil {
			return PrecisionDecimalFacetDeclarations{}, err
		}
	}
	return NewPrecisionDecimalFacetDeclarationsAll(
		totalDigits,
		minScale,
		maxScale,
		patterns,
		enumeration,
		minInclusive,
		minExclusive,
		maxInclusive,
		maxExclusive,
		whiteSpace,
	), nil
}

func schemaFacetValueLocation(input schemaFacetInput) Loc {
	if !input.valueLoc.IsZero() {
		return input.valueLoc
	}
	return input.loc
}

func unsupportedSchemaDatatypeFacet(input schemaFacetInput, version XSDVersion) error {
	if err := validateOrdinarySchemaFacetInput(input); err != nil {
		return err
	}
	if version == XSDVersion10 && isXSD11SimpleTypeFacet(schemaFacetName(input.kind)) {
		return newXSD11FeatureMismatch(
			FeatureDatatypeFacets,
			UnsupportedDatatypeFacetCode,
			input.loc,
			fmt.Sprintf("simple type restriction facet <%s> is an XSD 1.1-only construct", schemaFacetName(input.kind)),
		)
	}
	feature, ok := LookupUnsupportedFeature(FeatureDatatypeFacets)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			schemaFacetValueLocation(input),
			"datatype facet feature is not registered",
			nil,
		)
	}
	return newUnsupportedForVersion(
		feature,
		UnsupportedDatatypeFacetCode,
		input.loc,
		fmt.Sprintf("simple type restriction facet <%s> is not implemented for this datatype", schemaFacetName(input.kind)),
		version,
	)
}

func validateOrdinarySchemaFacetInput(input schemaFacetInput) error {
	valueLoc := schemaFacetValueLocation(input)
	switch input.kind {
	case schemaFacetMinScale, schemaFacetMaxScale:
		value, err := ParseStrictInteger(collapseXMLWhitespace(input.lexical), valueLoc)
		if err != nil {
			return err
		}
		if value.Sign() < 0 {
			return newSchemaCompositionDiagnostic(valueLoc, schemaFacetName(input.kind)+" facet value must be non-negative")
		}
	case schemaFacetPrecision:
		value, err := ParseStrictInteger(collapseXMLWhitespace(input.lexical), valueLoc)
		if err != nil {
			return err
		}
		if value.Sign() <= 0 {
			return newSchemaCompositionDiagnostic(valueLoc, "precision facet value must be positive")
		}
	case schemaFacetTotalDigits, schemaFacetFractionDigits, schemaFacetPattern, schemaFacetEnumeration,
		schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive,
		schemaFacetWhiteSpace, schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength,
		schemaFacetExplicitTimezone:
		return nil
	default:
		return newSchemaBridgeInvariant(input.loc, "simple type facet has an unknown kind")
	}
	return nil
}

func (resolver *schemaSimpleTypeResolver) cycleDiagnostic(input *schemaSimpleTypeInput, version XSDVersion) error {
	loc := simpleTypeInputReferenceLoc(input)
	start := 0
	found := false
	for position, current := range resolver.stack {
		if current == input {
			start = position
			found = true
			break
		}
	}
	if !found {
		return newSchemaBridgeInvariant(loc, "simple type cycle state is missing its stack entry")
	}
	if loc.IsZero() {
		loc = resolver.stackFallbackLoc[start]
	}
	related := make([]Loc, 0, len(resolver.stack)-start)
	for position, current := range resolver.stack[start:] {
		currentLoc := simpleTypeInputReferenceLoc(current)
		if currentLoc.IsZero() {
			fallbackIndex := start + position
			if fallbackIndex < len(resolver.stackFallbackLoc) {
				currentLoc = resolver.stackFallbackLoc[fallbackIndex]
			}
		}
		if currentLoc.IsZero() || currentLoc == loc {
			continue
		}
		related = append(related, currentLoc)
	}
	return newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeCycleCode,
		loc,
		"simple type definitions form a cycle",
		related,
		version,
		fmt.Errorf("%w: source %s", errSchemaSimpleTypeBaseCycle, loc.Source()),
	)
}

func simpleTypeInputReferenceLoc(input *schemaSimpleTypeInput) Loc {
	if input == nil {
		return Loc{}
	}
	model := input.model
	if model == nil {
		if !input.baseLoc.IsZero() {
			return input.baseLoc
		}
		return input.loc
	}
	switch typed := model.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if typed != nil && !typed.base.loc.IsZero() {
			return typed.base.loc
		}
	case *schemaSimpleTypeListModelInput:
		if typed != nil && !typed.itemType.loc.IsZero() {
			return typed.itemType.loc
		}
	case *schemaSimpleTypeUnionModelInput:
		if typed != nil && len(typed.members) > 0 && !typed.members[0].loc.IsZero() {
			return typed.members[0].loc
		}
	}
	return input.loc
}

func schemaComponentLocations(records []schemaComponentRecord, indices []int) []Loc {
	locations := make([]Loc, 0, len(indices))
	for _, index := range indices {
		if records[index].loc.IsZero() {
			continue
		}
		locations = append(locations, records[index].loc)
	}
	return locations
}

func newSchemaSimpleTypeDiagnostic(code string, loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaSimpleTypeSpecRef(version),
		cause:   cause,
	}
}

func schemaSimpleTypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaSimpleTypeXSD10SpecRef
	}
	return schemaSimpleTypeXSD11SpecRef
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

func newSchemaSyntaxUnsupportedForVersion(loc Loc, message string, version XSDVersion) Diagnostic {
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
	return newUnsupportedForVersion(feature, UnsupportedSchemaSyntaxCode, loc, message, version)
}

func newSchemaAnyAttributeUnsupported(loc Loc, version XSDVersion) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxFeatureCode,
			loc,
			"schema syntax feature is not registered",
			errSchemaAnyAttributeUnsupported,
		)
	}
	diagnostic := newUnsupportedForVersionWithCause(
		feature,
		UnsupportedSchemaSyntaxCode,
		loc,
		"anyAttribute wildcards are not implemented",
		version,
		errSchemaAnyAttributeUnsupported,
	)
	if diagnostic.Class() == FailureUnsupported {
		diagnostic.specRef = schemaAnyAttributeSpecRef(version)
	}
	return diagnostic
}

func schemaAnyAttributeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaAnyAttributeXSD10SpecRef
	}
	return schemaAnyAttributeXSD11SpecRef
}

func newXSD11FeatureMismatch(featureID FeatureID, code string, loc Loc, message string) Diagnostic {
	return newXSD11FeatureMismatchAtReference(featureID, code, loc, message, "", nil)
}

func newXSD11FeatureMismatchAtReference(featureID FeatureID, code string, loc Loc, message, specRef string, cause error) Diagnostic {
	feature, ok := LookupUnsupportedFeature(featureID)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			fmt.Sprintf("unsupported diagnostic references unregistered feature %q", featureID),
			cause,
		)
	}
	mismatchCause := fmt.Errorf("%w: feature %q is not available in the selected XSD 1.0 profile", errLanguagePolicyMismatch, featureID)
	if cause != nil {
		cause = errors.Join(cause, mismatchCause)
	}
	if cause == nil {
		cause = mismatchCause
	}
	diagnostic := newUnsupportedForVersionWithCause(feature, code, loc, message, XSDVersion11, cause)
	if diagnostic.Class() != FailureUnsupported || specRef == "" {
		return diagnostic
	}
	diagnostic.specRef = specRef
	return diagnostic
}
