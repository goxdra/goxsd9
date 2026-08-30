package goxsd9

import (
	"errors"
	"fmt"
	"go/format"
	"strconv"
	"strings"
)

const (
	codegenRuntimeImportPath       = "github.com/goxdra/goxsd9"
	diagnosticCodegenUnsupported   = "GOXSD9029"
	diagnosticCodegenInvariant     = "GOXSD9030"
	diagnosticCodegenFormat        = "GOXSD9031"
	diagnosticCodegenSchemaInvalid = "GOXSD9032"
	diagnosticCodegenNamingInvalid = "GOXSD9033"
	codegenElementDefaultVersion   = XSDVersion11
)

var (
	errCodegenSchemaEmpty      = errors.New("code-generation schema is zero or incomplete")
	errCodegenNamingMisaligned = errors.New("code-generation naming table does not match schema")
	errCodegenRuntimeImport    = errors.New("code-generation runtime import alias is missing")
	errCodegenUnsupported      = errors.New("schema behavior is outside scalar Go code generation")
	errCodegenElementType      = errors.New("global element type identity is incomplete")
	errCodegenSchemaInvariant  = errors.New("code-generation schema invariant is broken")
	errCodegenFormat           = errors.New("formatted Go source could not be produced")
)

type codegenSourcePlan struct {
	packageName  string
	runtimeAlias string
	useRuntime   bool
	declarations []codegenSourceDeclaration
}

type codegenSourceDeclaration struct {
	name      string
	fieldType string
	choice    *codegenSourceChoice
}

type codegenSourceChoice struct {
	marker   string
	variants []codegenSourceVariant
}

type codegenSourceVariant struct {
	name      string
	fieldName string
	fieldType string
}

// emitCodegenSource plans supported scalar declarations in schema order and
// returns one complete formatted Go source file. The result is nil on error.
func emitCodegenSource(schema Schema, names codegenNaming) ([]byte, error) {
	plan, err := planCodegenSource(schema, names)
	if err != nil {
		return nil, err
	}
	return renderCodegenSource(plan)
}

func emitCodegenSourceWithDirectChoices(schema Schema, choicePlan codegenDirectChoicePlan) ([]byte, error) {
	plan, err := planCodegenSourceWithDirectChoices(schema, choicePlan)
	if err != nil {
		return nil, err
	}
	return renderCodegenSource(plan)
}

// emitCodegen is the private phase boundary used by code-generation callers.
func emitCodegen(schema Schema, names codegenNaming) ([]byte, error) {
	return emitCodegenSource(schema, names)
}

func planCodegenSource(schema Schema, names codegenNaming) (codegenSourcePlan, error) {
	return planCodegenSourceWithChoicePlan(schema, names, nil)
}

func planCodegenSourceWithDirectChoices(schema Schema, choicePlan codegenDirectChoicePlan) (codegenSourcePlan, error) {
	if choicePlan.names.packageIdentifier() == "" {
		return codegenSourcePlan{}, newCodegenInternal(
			Loc{},
			"direct-choice source plan has no naming state",
			nil,
			errCodegenDirectChoiceNaming,
		)
	}
	if err := validateCodegenDirectChoicePlan(schema, choicePlan); err != nil {
		return codegenSourcePlan{}, err
	}
	return planCodegenSourceWithChoicePlan(schema, choicePlan.names, &choicePlan)
}

//nolint:gocognit // Keep ordered scalar and direct-choice declaration planning together.
func planCodegenSourceWithChoicePlan(
	schema Schema,
	names codegenNaming,
	choicePlan *codegenDirectChoicePlan,
) (codegenSourcePlan, error) {
	components, runtimeAlias, hasRuntimeAlias, err := validateCodegenInput(schema, names)
	if err != nil {
		return codegenSourcePlan{}, err
	}

	plan := codegenSourcePlan{
		packageName:  names.packageIdentifier(),
		runtimeAlias: runtimeAlias,
		declarations: make([]codegenSourceDeclaration, 0, len(components)),
	}
	policyVersion, versionErr := codegenSchemaVersion(schema)
	if versionErr != nil {
		return codegenSourcePlan{}, versionErr
	}
	if err := rejectCodegenElementFacts(components, policyVersion); err != nil {
		return codegenSourcePlan{}, err
	}
	for _, component := range components {
		identifier, ok := names.componentName(component.ID())
		if !ok {
			return codegenSourcePlan{}, newCodegenNamingInvariant(
				component.Loc(),
				fmt.Sprintf("component %s has no generated name", component.ID().Source()),
				errCodegenNamingMisaligned,
			)
		}
		if choicePlan != nil && component.Kind() == ComponentKindComplexTypeDefinition {
			owner, ownerOK := codegenDirectChoiceOwnerAt(choicePlan.owners, component.ID())
			if !ownerOK {
				return codegenSourcePlan{}, newCodegenInternal(
					component.Loc(),
					"schema complex type has no direct-choice source plan owner",
					nil,
					errCodegenDirectChoicePlan,
				)
			}
			choice, usesRuntime, choiceErr := planCodegenSourceChoice(owner, choicePlan.runtimeAlias)
			if choiceErr != nil {
				return codegenSourcePlan{}, choiceErr
			}
			plan.useRuntime = plan.useRuntime || usesRuntime
			plan.declarations = append(plan.declarations, codegenSourceDeclaration{
				name:   identifier,
				choice: &choice,
			})
			continue
		}
		fieldType, usesRuntime, err := planCodegenComponent(
			schema,
			names,
			component,
			runtimeAlias,
			hasRuntimeAlias,
			policyVersion,
			choicePlan != nil,
		)
		if err != nil {
			return codegenSourcePlan{}, err
		}
		plan.useRuntime = plan.useRuntime || usesRuntime
		plan.declarations = append(plan.declarations, codegenSourceDeclaration{
			name:      identifier,
			fieldType: fieldType,
		})
	}
	return plan, nil
}

func rejectCodegenElementFacts(components []Component, version XSDVersion) error {
	for _, component := range components {
		if component.Kind() != ComponentKindElementDeclaration {
			continue
		}
		declaration, ok := component.ElementDeclaration()
		if !ok {
			continue
		}
		if declaration.IsAbstract() {
			return newCodegenElementUnsupported(
				declaration.Loc(),
				fmt.Sprintf("global element %q has abstract=true outside Go generation", declaration.Name()),
				nil,
				fmt.Errorf("%w: non-default abstract element fact", errCodegenUnsupported),
				version,
			)
		}
		if declaration.IsNillable() {
			return newCodegenElementUnsupported(
				declaration.Loc(),
				fmt.Sprintf("global element %q has nillable=true outside Go generation", declaration.Name()),
				nil,
				fmt.Errorf("%w: non-default nillable element fact", errCodegenUnsupported),
				version,
			)
		}
	}
	return nil
}

func planCodegenSourceChoice(owner codegenDirectChoiceOwner, runtimeAlias string) (codegenSourceChoice, bool, error) {
	if owner.marker != codegenDirectChoiceMarkerIdentifier(owner.identifier) || !validCodegenDirectChoiceMarker(owner.marker) {
		return codegenSourceChoice{}, false, newCodegenInternal(
			owner.loc,
			"direct-choice owner marker is malformed",
			nil,
			errCodegenDirectChoicePlan,
		)
	}
	choice := codegenSourceChoice{
		marker:   owner.marker,
		variants: make([]codegenSourceVariant, 0, len(owner.alternatives)),
	}
	usesRuntime := false
	for _, alternative := range owner.alternatives {
		fieldType, alternativeUsesRuntime, err := planCodegenSourceChoiceTarget(
			alternative.target,
			runtimeAlias,
			alternative.loc,
		)
		if err != nil {
			return codegenSourceChoice{}, false, err
		}
		usesRuntime = usesRuntime || alternativeUsesRuntime
		choice.variants = append(choice.variants, codegenSourceVariant{
			name:      alternative.variantIdentifier,
			fieldName: alternative.fieldIdentifier,
			fieldType: fieldType,
		})
	}
	return choice, usesRuntime, nil
}

func planCodegenSourceChoiceTarget(target codegenDirectChoiceTarget, runtimeAlias string, loc Loc) (string, bool, error) {
	switch concrete := target.(type) {
	case codegenDirectChoiceBuiltinTarget:
		fieldType, err := codegenRuntimeScalarType(runtimeAlias, runtimeAlias != "", concrete.kind, loc)
		return fieldType, true, err
	case codegenDirectChoiceNamedTarget:
		if concrete.componentIdentifier == "" {
			return "", false, newCodegenInternal(
				loc,
				"direct-choice named target has no generated component identifier",
				nil,
				errCodegenDirectChoicePlan,
			)
		}
		return concrete.componentIdentifier, false, nil
	default:
		return "", false, newCodegenInternal(
			loc,
			"direct-choice source target has an unknown representation",
			nil,
			errCodegenDirectChoicePlan,
		)
	}
}

func codegenSchemaVersion(schema Schema) (XSDVersion, error) {
	if schema.policy == "" {
		return codegenElementDefaultVersion, nil
	}
	version, err := xsdVersionForLanguagePolicy(schema.policy)
	if err != nil {
		return "", newCodegenSchemaInvariant(
			Loc{},
			"schema has an invalid language policy",
			err,
		)
	}
	return version, nil
}

func planCodegenComponent(
	schema Schema,
	names codegenNaming,
	component Component,
	runtimeAlias string,
	hasRuntimeAlias bool,
	policyVersion XSDVersion,
	allowComplexElementType bool,
) (string, bool, error) {
	switch component.Kind() {
	case ComponentKindSimpleTypeDefinition:
		kind, err := codegenNamedScalarKind(component)
		if err != nil {
			return "", false, err
		}
		fieldType, err := codegenRuntimeScalarType(runtimeAlias, hasRuntimeAlias, kind, component.Loc())
		if err != nil {
			return "", false, err
		}
		return fieldType, true, nil
	case ComponentKindElementDeclaration:
		return codegenElementFieldType(schema, names, component, runtimeAlias, hasRuntimeAlias, policyVersion, allowComplexElementType)
	case ComponentKindAttributeDeclaration,
		ComponentKindComplexTypeDefinition,
		ComponentKindModelGroupDefinition,
		ComponentKindAttributeGroupDefinition,
		ComponentKindNotationDeclaration:
		return "", false, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("schema component kind %q is not supported by Go scalar generation", component.Kind()),
			nil,
			fmt.Errorf("%w: component kind %q", errCodegenUnsupported, component.Kind()),
			"",
		)
	default:
		return "", false, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("unknown schema component kind %q is not supported by Go scalar generation", component.Kind()),
			nil,
			fmt.Errorf("%w: unknown component kind %q", errCodegenUnsupported, component.Kind()),
			"",
		)
	}
}

func validateCodegenInput(schema Schema, names codegenNaming) ([]Component, string, bool, error) {
	if err := validateCodegenPackageName(names.packageIdentifier()); err != nil {
		return nil, "", false, err
	}
	if schema.storage == nil {
		return nil, "", false, newDiagnostic(
			FailureInvalid,
			diagnosticCodegenSchemaInvalid,
			Loc{},
			"code-generation schema is zero or incomplete",
			errCodegenSchemaEmpty,
		)
	}

	components := schema.Components()
	if err := validateCodegenSchemaStorage(schema, components); err != nil {
		return nil, "", false, err
	}
	if err := validateCodegenNaming(components, names); err != nil {
		return nil, "", false, err
	}
	runtimeAlias, hasRuntimeAlias := codegenRuntimeAlias(names)
	return components, runtimeAlias, hasRuntimeAlias, nil
}

func validateCodegenSchemaStorage(schema Schema, components []Component) error {
	if len(schema.storage.byID) != len(components) {
		return newCodegenSchemaInvariant(
			Loc{},
			"completed schema component identity index has the wrong size",
			errCodegenSchemaInvariant,
		)
	}
	seen := make(map[ComponentID]struct{}, len(components))
	for index, component := range components {
		if component.ID().IsZero() || component.ID().Source() == "" || component.ID().Ordinal() == 0 {
			return newCodegenSchemaInvariant(component.Loc(), "schema component has an incomplete identity", errCodegenSchemaInvariant)
		}
		if _, exists := seen[component.ID()]; exists {
			return newCodegenSchemaInvariant(component.Loc(), "schema component identity is repeated", errCodegenSchemaInvariant)
		}
		seen[component.ID()] = struct{}{}
		if component.Kind() == "" || component.Name().IsZero() {
			return newCodegenSchemaInvariant(component.Loc(), "schema component has incomplete declaration facts", errCodegenSchemaInvariant)
		}
		storedIndex, ok := schema.storage.byID[component.ID()]
		if !ok || storedIndex != index {
			return newCodegenSchemaInvariant(
				component.Loc(),
				fmt.Sprintf("schema identity index does not point to component %s", component.ID().Source()),
				errCodegenSchemaInvariant,
			)
		}
	}
	return nil
}

func validateCodegenNaming(components []Component, names codegenNaming) error {
	if len(names.components) != len(components) {
		return newCodegenNamingInvariant(
			Loc{},
			"naming table component count does not match schema",
			errCodegenNamingMisaligned,
		)
	}
	if len(names.componentByID) != len(names.components) {
		return newCodegenNamingInvariant(
			Loc{},
			"naming table component index has the wrong size",
			errCodegenNamingMisaligned,
		)
	}

	allocated := make(map[string]struct{}, len(names.components)+len(names.imports))
	if err := validateCodegenComponentNames(components, names, allocated); err != nil {
		return err
	}
	return validateCodegenImportNames(names, allocated)
}

func validateCodegenComponentNames(components []Component, names codegenNaming, allocated map[string]struct{}) error {
	for index, component := range components {
		record := names.components[index]
		if record.id != component.ID() || record.kind != component.Kind() || record.name != component.Name() {
			return newCodegenNamingInvariant(component.Loc(), "naming table component record does not match schema", errCodegenNamingMisaligned)
		}
		identifier, ok := names.componentName(component.ID())
		if !ok || identifier != record.identifier {
			return newCodegenNamingInvariant(component.Loc(), "naming table component lookup is stale", errCodegenNamingMisaligned)
		}
		if err := validateCodegenAllocatedIdentifier(identifier, codegenNameKindComponent, "component"); err != nil {
			return err
		}
		key := codegenCaseFold(identifier)
		if _, exists := allocated[key]; exists {
			return newCodegenNamingInvariant(component.Loc(), "naming table contains a repeated package identifier", errCodegenNamingMisaligned)
		}
		allocated[key] = struct{}{}
	}
	return nil
}

func validateCodegenImportNames(names codegenNaming, allocated map[string]struct{}) error {
	if len(names.importByID) != len(names.imports) {
		return newCodegenNamingInvariant(Loc{}, "naming table import index has the wrong size", errCodegenNamingMisaligned)
	}
	allocator := &codegenNameAllocator{used: make(map[string]struct{}, len(allocated))}
	for key := range allocated {
		allocator.used[key] = struct{}{}
	}
	for _, imported := range names.imports {
		if err := validateCodegenImportName(names, imported, allocator, allocated); err != nil {
			return err
		}
	}

	return nil
}

func validateCodegenImportName(
	names codegenNaming,
	imported codegenImportAliasName,
	allocator *codegenNameAllocator,
	allocated map[string]struct{},
) error {
	if err := validateCodegenImportAliasRequest(codegenImportAliasRequest{
		identity: imported.identity,
		alias:    imported.alias,
	}); err != nil {
		return err
	}
	requested, err := codegenIdentifier(imported.alias, codegenNameKindImport, false, nil, Loc{})
	if err != nil {
		return err
	}
	if err := validateCodegenAllocatedIdentifier(imported.identifier, codegenNameKindImport, "import alias"); err != nil {
		return err
	}
	identifier, allocationErr := allocator.allocate(requested)
	if allocationErr != nil {
		return newCodegenNamingInvariant(Loc{}, "could not replay import alias allocation", allocationErr)
	}
	if identifier != imported.identifier {
		return newCodegenNamingInvariant(Loc{}, "naming table import alias is not allocated from its request", errCodegenNamingMisaligned)
	}
	indexed, ok := names.importAlias(imported.identity)
	if !ok || indexed != imported.identifier {
		return newCodegenNamingInvariant(Loc{}, "naming table import lookup is stale", errCodegenNamingMisaligned)
	}
	key := codegenCaseFold(imported.identifier)
	if _, exists := allocated[key]; exists {
		return newCodegenNamingInvariant(Loc{}, "naming table contains a repeated import identifier", errCodegenNamingMisaligned)
	}
	allocated[key] = struct{}{}
	return nil
}

func validateCodegenAllocatedIdentifier(identifier string, kind codegenNameKind, context string) error {
	normalized, err := codegenIdentifier(identifier, kind, false, nil, Loc{})
	if err != nil {
		return err
	}
	if normalized != identifier {
		return newCodegenNameError(Loc{}, context, identifier, "the naming table contains an unallocated identifier")
	}
	return nil
}

func codegenRuntimeAlias(names codegenNaming) (string, bool) {
	for _, imported := range names.imports {
		if imported.identity == codegenRuntimeImportPath {
			return imported.identifier, true
		}
	}
	return "", false
}

func codegenNamedScalarKind(component Component) (DigitDatatype, error) {
	definition, ok := component.SimpleTypeDefinition()
	if !ok {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has no supported simple-type view", component.Name()),
			nil,
			fmt.Errorf("%w: simple type view is missing", errCodegenUnsupported),
			"",
		)
	}
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has variety %q outside scalar Go generation", component.Name(), definition.Variety()),
			appendCodegenRelated(nil, definition.VarietyLoc()),
			fmt.Errorf("%w: simple type variety %q", errCodegenUnsupported, definition.Variety()),
			codegenElementDefaultVersion,
		)
	}
	if definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicInteger && definition.facts.atomicKind != schemaSimpleTypeAtomicDecimal {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has an unsupported atomic datatype", component.Name()),
			appendCodegenRelated(nil, definition.BaseLoc()),
			fmt.Errorf("%w: atomic datatype is outside scalar Go generation", errCodegenUnsupported),
			codegenElementDefaultVersion,
		)
	}
	facets := definition.DigitFacets()
	if facets.Kind() != DigitDatatypeInteger && facets.Kind() != DigitDatatypeDecimal {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has unsupported digit datatype %q", component.Name(), facets.Kind()),
			codegenSimpleTypeRelatedLocations(definition, facets),
			fmt.Errorf("%w: digit datatype %q", errCodegenUnsupported, facets.Kind()),
			facets.Version(),
		)
	}
	if err := facets.validate(); err != nil {
		return "", newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("validate effective digit facets for named simple type %q", component.Name()),
			codegenSimpleTypeRelatedLocations(definition, facets),
			err,
		)
	}
	return facets.Kind(), nil
}

func codegenSimpleTypeRelatedLocations(definition SimpleTypeDefinition, facets DigitFacets) []Loc {
	locations := make([]Loc, 0, 3)
	locations = appendCodegenRelated(locations, definition.BaseLoc())
	if loc, ok := facets.TotalDigitsLoc(); ok {
		locations = appendCodegenRelated(locations, loc)
	}
	if loc, ok := facets.FractionDigitsLoc(); ok {
		locations = appendCodegenRelated(locations, loc)
	}
	return locations
}

func codegenRuntimeScalarType(alias string, hasAlias bool, kind DigitDatatype, loc Loc) (string, error) {
	if !hasAlias || alias == "" {
		return "", newCodegenNameError(
			loc,
			"runtime import alias",
			codegenRuntimeImportPath,
			"the ordered runtime import request is missing",
		)
	}
	switch kind {
	case DigitDatatypeInteger:
		return alias + ".StrictInteger", nil
	case DigitDatatypeDecimal:
		return alias + ".StrictDecimal", nil
	default:
		return "", newCodegenInternal(
			loc,
			fmt.Sprintf("render unknown scalar digit datatype %q", kind),
			nil,
			fmt.Errorf("%w: unknown digit datatype %q", errCodegenSchemaInvariant, kind),
		)
	}
}

func codegenElementFieldType(
	schema Schema,
	names codegenNaming,
	component Component,
	runtimeAlias string,
	hasRuntimeAlias bool,
	version XSDVersion,
	allowComplexElementType bool,
) (string, bool, error) {
	declaration, ok := component.ElementDeclaration()
	if !ok {
		return "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element %q has no supported declaration view", component.Name()),
			nil,
			fmt.Errorf("%w: element declaration view is missing", errCodegenUnsupported),
			version,
		)
	}
	declaredType := declaration.DeclaredType()
	if declaredType.IsZero() {
		return "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element %q has no explicit scalar type", component.Name()),
			nil,
			fmt.Errorf("%w: declared type is empty", errCodegenUnsupported),
			version,
		)
	}
	if declaredType.Namespace() == xsdNamespaceURI {
		return codegenBuiltinElementFieldType(component, declaration, runtimeAlias, hasRuntimeAlias, version)
	}
	return codegenNamedElementFieldType(schema, names, component, declaration, version, allowComplexElementType)
}

func codegenBuiltinElementFieldType(
	component Component,
	declaration ElementDeclaration,
	runtimeAlias string,
	hasRuntimeAlias bool,
	version XSDVersion,
) (string, bool, error) {
	typeID, hasTypeID := declaration.TypeID()
	if hasTypeID || !typeID.IsZero() {
		return "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("built-in global element %q has a synthetic type identity", component.Name()),
			nil,
			errCodegenElementType,
		)
	}
	switch declaration.DeclaredType().Local() {
	case "integer":
		fieldType, err := codegenRuntimeScalarType(runtimeAlias, hasRuntimeAlias, DigitDatatypeInteger, component.Loc())
		return fieldType, true, err
	case "decimal":
		fieldType, err := codegenRuntimeScalarType(runtimeAlias, hasRuntimeAlias, DigitDatatypeDecimal, component.Loc())
		return fieldType, true, err
	default:
		return "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element type %q is outside scalar Go generation", declaration.DeclaredType()),
			nil,
			fmt.Errorf("%w: built-in type %q", errCodegenUnsupported, declaration.DeclaredType()),
			version,
		)
	}
}

func codegenNamedElementFieldType(
	schema Schema,
	names codegenNaming,
	component Component,
	declaration ElementDeclaration,
	version XSDVersion,
	allowComplexElementType bool,
) (string, bool, error) {
	declaredType := declaration.DeclaredType()
	typeID, hasTypeID := declaration.TypeID()
	if !hasTypeID || typeID.IsZero() {
		return "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("named global element %q has no resolved type identity", component.Name()),
			nil,
			errCodegenElementType,
		)
	}
	target, ok := schema.Lookup(typeID)
	if !ok {
		return "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("global element type identity %s is absent from the schema", typeID.Source()),
			nil,
			errCodegenElementType,
		)
	}
	related := appendCodegenRelated(nil, target.Loc())
	if target.ID() != typeID || target.Name() != declaredType {
		return "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("global element %q type identity does not match its declared QName", component.Name()),
			related,
			errCodegenElementType,
		)
	}
	if target.Kind() == ComponentKindComplexTypeDefinition && allowComplexElementType {
		identifier, nameOK := names.componentName(typeID)
		if !nameOK {
			return "", false, newCodegenNamingInvariant(
				component.Loc(),
				fmt.Sprintf("named global element type %s has no generated name", typeID.Source()),
				errCodegenNamingMisaligned,
			)
		}
		return identifier, false, nil
	}
	if target.Kind() != ComponentKindSimpleTypeDefinition {
		return "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element type %q is not a supported named simple type", declaredType),
			related,
			fmt.Errorf("%w: target component kind %q", errCodegenUnsupported, target.Kind()),
			version,
		)
	}
	_, err := codegenNamedScalarKind(target)
	if err != nil {
		return "", false, decorateCodegenElementError(err, component.Loc(), related)
	}
	identifier, ok := names.componentName(typeID)
	if !ok {
		return "", false, newCodegenNamingInvariant(
			component.Loc(),
			fmt.Sprintf("named global element type %s has no generated name", typeID.Source()),
			errCodegenNamingMisaligned,
		)
	}
	return identifier, false, nil
}

func decorateCodegenElementError(err error, loc Loc, related []Loc) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return newCodegenInternal(loc, "classify named global element type failure", related, err)
	}
	decorated := diagnostic
	decorated.loc = loc
	decorated.related = mergeCodegenRelated(related, diagnostic.related)
	return decorated
}

func renderCodegenSource(plan codegenSourcePlan) ([]byte, error) {
	var source strings.Builder
	source.WriteString("package ")
	source.WriteString(plan.packageName)
	source.WriteString("\n\n")
	if plan.useRuntime {
		if plan.runtimeAlias == "" {
			return nil, newCodegenInternal(
				Loc{},
				"render scalar source without a runtime import alias",
				nil,
				errCodegenRuntimeImport,
			)
		}
		source.WriteString("import ")
		source.WriteString(plan.runtimeAlias)
		source.WriteByte(' ')
		source.WriteString(strconv.Quote(codegenRuntimeImportPath))
		source.WriteString("\n\n")
	}
	for _, declaration := range plan.declarations {
		if declaration.choice != nil {
			if err := renderCodegenChoiceDeclaration(&source, declaration); err != nil {
				return nil, err
			}
			continue
		}
		source.WriteString("type ")
		source.WriteString(declaration.name)
		source.WriteString(" struct {\n\tValue ")
		source.WriteString(declaration.fieldType)
		source.WriteString("\n}\n\n")
	}

	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return nil, newCodegenFormat(Loc{}, "format generated Go source", fmt.Errorf("%w: %w", errCodegenFormat, err))
	}
	return formatted, nil
}

func renderCodegenChoiceDeclaration(source *strings.Builder, declaration codegenSourceDeclaration) error {
	choice := declaration.choice
	if !validCodegenDirectChoiceMarker(choice.marker) {
		return newCodegenInternal(
			Loc{},
			"render direct-choice declaration with an invalid marker",
			nil,
			errCodegenDirectChoicePlan,
		)
	}
	source.WriteString("type ")
	source.WriteString(declaration.name)
	source.WriteString(" interface {\n\t")
	source.WriteString(choice.marker)
	source.WriteString("()\n}\n\n")
	for _, variant := range choice.variants {
		if variant.name == "" || variant.fieldName == "" || variant.fieldType == "" {
			return newCodegenInternal(
				Loc{},
				"render direct-choice declaration with incomplete variant facts",
				nil,
				errCodegenDirectChoicePlan,
			)
		}
		source.WriteString("type ")
		source.WriteString(variant.name)
		source.WriteString(" struct {\n\t")
		source.WriteString(variant.fieldName)
		source.WriteByte(' ')
		source.WriteString(variant.fieldType)
		source.WriteString("\n}\n\nfunc (")
		source.WriteString(variant.name)
		source.WriteString(") ")
		source.WriteString(choice.marker)
		source.WriteString("() {}\n\n")
	}
	return nil
}

func validCodegenDirectChoiceMarker(marker string) bool {
	return strings.HasPrefix(marker, "is") && isGoIdentifier(marker)
}

func newCodegenNamingInvariant(loc Loc, message string, cause error) Diagnostic {
	return newDiagnostic(FailureInvalid, diagnosticCodegenNamingInvalid, loc, message, cause)
}

func newCodegenSchemaInvariant(loc Loc, message string, cause error) Diagnostic {
	return newDiagnostic(FailureInternal, diagnosticCodegenInvariant, loc, message, cause)
}

func newCodegenInternal(loc Loc, message string, related []Loc, cause error) Diagnostic {
	diagnostic := newDiagnostic(FailureInternal, diagnosticCodegenInvariant, loc, message, cause)
	diagnostic.related = append([]Loc(nil), related...)
	return diagnostic
}

func newCodegenFormat(loc Loc, message string, cause error) Diagnostic {
	return newDiagnostic(FailureInternal, diagnosticCodegenFormat, loc, message, cause)
}

func newCodegenUnsupported(loc Loc, message string, related []Loc, cause error, version XSDVersion) error {
	specRef := ""
	if version == XSDVersion10 || version == XSDVersion11 {
		specRef = schemaSimpleTypeSpecRef(version)
	}
	return newCodegenUnsupportedForReference(loc, message, related, cause, version, specRef)
}

func newCodegenElementUnsupported(loc Loc, message string, related []Loc, cause error, version XSDVersion) error {
	specRef := ""
	if version == XSDVersion10 || version == XSDVersion11 {
		specRef = schemaElementTypeSpecRef(version)
	}
	return newCodegenUnsupportedForReference(
		loc,
		message,
		related,
		cause,
		version,
		specRef,
	)
}

func newCodegenUnsupportedForReference(
	loc Loc,
	message string,
	related []Loc,
	cause error,
	version XSDVersion,
	specRef string,
) error {
	feature, ok := LookupUnsupportedFeature(FeatureCodegen)
	if !ok {
		diagnostic := newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			"code-generation unsupported feature is not registered",
			cause,
		)
		diagnostic.related = append([]Loc(nil), related...)
		return diagnostic
	}
	if version != XSDVersion10 && version != XSDVersion11 {
		return decorateCodegenUnsupported(
			newUnsupported(feature, diagnosticCodegenUnsupported, loc, message),
			related,
			cause,
		)
	}
	diagnostic := newUnsupportedForVersion(feature, diagnosticCodegenUnsupported, loc, message, version)
	if specRef != "" {
		for _, reference := range feature.References() {
			if reference.Version() == string(version) && reference.Source() == specRef {
				diagnostic.specRef = reference.Source()
				break
			}
		}
	}
	return decorateCodegenUnsupported(diagnostic, related, cause)
}

func decorateCodegenUnsupported(diagnostic Diagnostic, related []Loc, cause error) error {
	if diagnostic.Class() != FailureUnsupported {
		diagnostic.cause = cause
		diagnostic.related = append([]Loc(nil), related...)
		return diagnostic
	}
	diagnostic.related = append([]Loc(nil), related...)
	diagnostic.cause = cause
	return diagnostic
}

func appendCodegenRelated(locations []Loc, loc Loc) []Loc {
	if loc.IsZero() {
		return locations
	}
	for _, existing := range locations {
		if existing == loc {
			return locations
		}
	}
	return append(locations, loc)
}

func mergeCodegenRelated(primary, additional []Loc) []Loc {
	result := make([]Loc, 0, len(primary)+len(additional))
	for _, loc := range primary {
		result = appendCodegenRelated(result, loc)
	}
	for _, loc := range additional {
		result = appendCodegenRelated(result, loc)
	}
	return result
}
