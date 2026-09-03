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
	packageName     string
	runtimeAlias    string
	useRuntime      bool
	directChoices   bool
	directSequences bool
	directParticles bool
	names           codegenNaming
	declarations    []codegenSourceDeclaration
}

type codegenSourceDeclaration struct {
	id          ComponentID
	schemaName  QName
	loc         Loc
	name        string
	fieldType   string
	usesRuntime bool
	target      codegenSourceTarget
	choice      *codegenSourceChoice
	sequence    *codegenSourceSequence
}

type codegenSourceChoice struct {
	ownerID     ComponentID
	ownerName   QName
	ownerLoc    Loc
	choiceLoc   Loc
	marker      string
	usesRuntime bool
	variants    []codegenSourceVariant
}

type codegenSourceVariant struct {
	path        []uint32
	loc         Loc
	schemaName  QName
	name        string
	fieldName   string
	fieldType   string
	usesRuntime bool
	target      codegenSourceTarget
}

type codegenSourceSequence struct {
	ownerID     ComponentID
	ownerName   QName
	ownerLoc    Loc
	sequenceLoc Loc
	usesRuntime bool
	fields      []codegenSourceSequenceField
}

type codegenSourceSequenceField struct {
	path        []uint32
	loc         Loc
	schemaName  QName
	fieldName   string
	fieldType   string
	usesRuntime bool
	target      codegenSourceTarget
}

type codegenSourceTargetForm uint8

const (
	codegenSourceTargetInvalid codegenSourceTargetForm = iota
	codegenSourceTargetDefinition
	codegenSourceTargetBuiltin
	codegenSourceTargetNamed
)

type codegenSourceScalarKind uint8

const (
	codegenSourceScalarInvalid codegenSourceScalarKind = iota
	codegenSourceScalarBoolean
	codegenSourceScalarInteger
	codegenSourceScalarDecimal
)

type codegenSourceTarget struct {
	form         codegenSourceTargetForm
	declaredType QName
	typeID       ComponentID
	hasTypeID    bool
	scalarKind   codegenSourceScalarKind
}

// emitCodegenSource plans supported scalar declarations in schema order and
// returns one complete formatted Go source file. The result is nil on error.
func emitCodegenSource(schema Schema, names codegenNaming) ([]byte, error) {
	plan, err := planCodegenSource(schema, names)
	if err != nil {
		return nil, err
	}
	return renderCodegenSource(plan, schema)
}

func emitCodegenSourceWithDirectChoices(schema Schema, choicePlan codegenDirectChoicePlan) ([]byte, error) {
	plan, err := planCodegenSourceWithDirectChoices(schema, choicePlan)
	if err != nil {
		return nil, err
	}
	return renderCodegenSource(plan, schema)
}

func emitCodegenSourceWithDirectParticles(schema Schema, directPlan codegenDirectParticlePlan) ([]byte, error) {
	plan, err := planCodegenSourceWithDirectParticles(schema, directPlan)
	if err != nil {
		return nil, err
	}
	return renderCodegenSource(plan, schema)
}

// emitCodegen is the private phase boundary used by code-generation callers.
func emitCodegen(schema Schema, names codegenNaming) ([]byte, error) {
	return emitCodegenSource(schema, names)
}

func prepareCodegenPlanSchema(schema Schema, packageName, emptyMessage string) ([]Component, XSDVersion, error) {
	if err := validateCodegenPackageName(packageName); err != nil {
		return nil, "", err
	}
	if schema.storage == nil {
		return nil, "", newDiagnostic(
			FailureInvalid,
			diagnosticCodegenSchemaInvalid,
			Loc{},
			emptyMessage,
			errCodegenSchemaEmpty,
		)
	}
	components := schema.Components()
	if err := validateCodegenSchemaStorage(schema, components); err != nil {
		return nil, "", err
	}
	version, err := codegenSchemaVersion(schema)
	if err != nil {
		return nil, "", err
	}
	if err := rejectCodegenElementFacts(components, version); err != nil {
		return nil, "", err
	}
	return components, version, nil
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

func planCodegenSourceWithDirectParticles(schema Schema, directPlan codegenDirectParticlePlan) (codegenSourcePlan, error) {
	if directPlan.names.packageIdentifier() == "" {
		return codegenSourcePlan{}, newCodegenInternal(
			codegenDirectParticlePlanLoc(directPlan),
			"direct-particle source plan has no naming state",
			nil,
			errCodegenDirectParticlePlan,
		)
	}
	if err := validateCodegenDirectParticlePlan(schema, directPlan); err != nil {
		return codegenSourcePlan{}, err
	}
	return planCodegenSourceWithDirectParticlePlan(schema, directPlan.names, nil, &directPlan)
}

func planCodegenSourceWithChoicePlan(
	schema Schema,
	names codegenNaming,
	choicePlan *codegenDirectChoicePlan,
) (codegenSourcePlan, error) {
	return planCodegenSourceWithDirectParticlePlan(schema, names, choicePlan, nil)
}

//nolint:gocognit,funlen // Keep ordered scalar and direct-particle declaration planning together.
func planCodegenSourceWithDirectParticlePlan(
	schema Schema,
	names codegenNaming,
	choicePlan *codegenDirectChoicePlan,
	directPlan *codegenDirectParticlePlan,
) (codegenSourcePlan, error) {
	components, runtimeAlias, hasRuntimeAlias, err := validateCodegenInput(schema, names)
	if err != nil {
		return codegenSourcePlan{}, err
	}

	plan := codegenSourcePlan{
		packageName:     names.packageIdentifier(),
		runtimeAlias:    runtimeAlias,
		directChoices:   choicePlan != nil,
		directParticles: directPlan != nil,
		names:           names.clone(),
		declarations:    make([]codegenSourceDeclaration, 0, len(components)),
	}
	if directPlan != nil {
		for _, owner := range directPlan.owners {
			if owner.kind == codegenDirectParticleSequence {
				plan.directSequences = true
			}
			if owner.kind == codegenDirectParticleChoice {
				plan.directChoices = true
			}
		}
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
		if directPlan != nil && component.Kind() == ComponentKindComplexTypeDefinition {
			owner, ownerOK := codegenDirectParticleOwnerAt(directPlan.owners, component.ID())
			if !ownerOK {
				return codegenSourcePlan{}, newCodegenInternal(
					component.Loc(),
					"schema complex type has no direct-particle source plan owner",
					nil,
					errCodegenDirectParticlePlan,
				)
			}
			if owner.kind == codegenDirectParticleChoice {
				if owner.choice == nil {
					return codegenSourcePlan{}, newCodegenInternal(
						component.Loc(),
						"direct-particle choice owner has no choice facts",
						nil,
						errCodegenDirectParticlePlan,
					)
				}
				choice, usesRuntime, choiceErr := planCodegenSourceChoice(*owner.choice, directPlan.runtimeAlias)
				if choiceErr != nil {
					return codegenSourcePlan{}, choiceErr
				}
				plan.useRuntime = plan.useRuntime || usesRuntime
				plan.declarations = append(plan.declarations, codegenSourceDeclaration{
					id:          component.ID(),
					schemaName:  component.Name(),
					loc:         component.Loc(),
					name:        identifier,
					usesRuntime: usesRuntime,
					choice:      &choice,
				})
				continue
			}
			if owner.kind == codegenDirectParticleSequence {
				if owner.sequence == nil {
					return codegenSourcePlan{}, newCodegenInternal(
						component.Loc(),
						"direct-particle sequence owner has no sequence facts",
						nil,
						errCodegenDirectParticlePlan,
					)
				}
				sequence, usesRuntime, sequenceErr := planCodegenSourceSequence(*owner.sequence, names, directPlan.runtimeAlias)
				if sequenceErr != nil {
					return codegenSourcePlan{}, sequenceErr
				}
				plan.useRuntime = plan.useRuntime || usesRuntime
				plan.declarations = append(plan.declarations, codegenSourceDeclaration{
					id:          component.ID(),
					schemaName:  component.Name(),
					loc:         component.Loc(),
					name:        identifier,
					usesRuntime: usesRuntime,
					sequence:    &sequence,
				})
				continue
			}
			return codegenSourcePlan{}, newCodegenInternal(
				component.Loc(),
				"direct-particle source plan has an unknown declaration shape",
				nil,
				errCodegenDirectParticlePlan,
			)
		}
		if component.Kind() == ComponentKindComplexTypeDefinition {
			if choicePlan == nil {
				return codegenSourcePlan{}, newCodegenInternal(
					component.Loc(),
					"direct-particle source plan has no complex-type mode",
					nil,
					errCodegenDirectParticlePlan,
				)
			}
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
				id:          component.ID(),
				schemaName:  component.Name(),
				loc:         component.Loc(),
				name:        identifier,
				usesRuntime: usesRuntime,
				choice:      &choice,
			})
			continue
		}
		target, fieldType, usesRuntime, err := planCodegenComponent(
			schema,
			names,
			component,
			runtimeAlias,
			hasRuntimeAlias,
			policyVersion,
			choicePlan != nil || directPlan != nil,
		)
		if err != nil {
			return codegenSourcePlan{}, err
		}
		plan.useRuntime = plan.useRuntime || usesRuntime
		plan.declarations = append(plan.declarations, codegenSourceDeclaration{
			id:          component.ID(),
			schemaName:  component.Name(),
			loc:         component.Loc(),
			name:        identifier,
			fieldType:   fieldType,
			usesRuntime: usesRuntime,
			target:      target,
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
		ownerID:   owner.id,
		ownerName: owner.name,
		ownerLoc:  owner.loc,
		choiceLoc: owner.choiceLoc,
		marker:    owner.marker,
		variants:  make([]codegenSourceVariant, 0, len(owner.alternatives)),
	}
	usesRuntime := false
	for _, alternative := range owner.alternatives {
		target, fieldType, alternativeUsesRuntime, err := planCodegenSourceChoiceTarget(
			alternative.target,
			runtimeAlias,
			alternative.loc,
		)
		if err != nil {
			return codegenSourceChoice{}, false, err
		}
		usesRuntime = usesRuntime || alternativeUsesRuntime
		choice.variants = append(choice.variants, codegenSourceVariant{
			path:        cloneCodegenPath(alternative.path),
			loc:         alternative.loc,
			schemaName:  alternative.name,
			name:        alternative.variantIdentifier,
			fieldName:   alternative.fieldIdentifier,
			fieldType:   fieldType,
			usesRuntime: alternativeUsesRuntime,
			target:      target,
		})
	}
	choice.usesRuntime = usesRuntime
	return choice, usesRuntime, nil
}

func planCodegenSourceSequence(
	owner codegenDirectSequenceOwner,
	names codegenNaming,
	runtimeAlias string,
) (codegenSourceSequence, bool, error) {
	sequence := codegenSourceSequence{
		ownerID:     owner.id,
		ownerName:   owner.name,
		ownerLoc:    owner.loc,
		sequenceLoc: owner.sequenceLoc,
		fields:      make([]codegenSourceSequenceField, 0, len(owner.fields)),
	}
	usesRuntime := false
	for _, field := range owner.fields {
		fieldType, fieldUsesRuntime, err := codegenSourceTargetFieldType(
			field.target,
			names,
			runtimeAlias,
			runtimeAlias != "",
			field.loc,
		)
		if err != nil {
			return codegenSourceSequence{}, false, err
		}
		usesRuntime = usesRuntime || fieldUsesRuntime
		sequence.fields = append(sequence.fields, codegenSourceSequenceField{
			path:        cloneCodegenPath(field.path),
			loc:         field.loc,
			schemaName:  field.name,
			fieldName:   field.fieldIdentifier,
			fieldType:   fieldType,
			usesRuntime: fieldUsesRuntime,
			target:      field.target,
		})
	}
	sequence.usesRuntime = usesRuntime
	return sequence, usesRuntime, nil
}

func planCodegenSourceChoiceTarget(
	target codegenDirectChoiceTarget,
	runtimeAlias string,
	loc Loc,
) (codegenSourceTarget, string, bool, error) {
	switch concrete := target.(type) {
	case codegenDirectChoiceBuiltinTarget:
		scalarKind, ok := codegenSourceScalarKindFromDigit(concrete.kind)
		if !ok {
			return codegenSourceTarget{}, "", false, newCodegenInternal(
				loc,
				"direct-choice source target has an unknown primitive kind",
				nil,
				errCodegenDirectChoicePlan,
			)
		}
		sourceTarget := codegenSourceTarget{
			form:         codegenSourceTargetBuiltin,
			declaredType: concrete.declaredType,
			scalarKind:   scalarKind,
		}
		fieldType, usesRuntime, err := codegenSourceTargetFieldType(
			sourceTarget,
			codegenNaming{},
			runtimeAlias,
			runtimeAlias != "",
			loc,
		)
		if err != nil {
			return codegenSourceTarget{}, "", false, err
		}
		return sourceTarget, fieldType, usesRuntime, nil
	case codegenDirectChoiceNamedTarget:
		if concrete.componentIdentifier == "" || concrete.id.IsZero() || concrete.declaredType.IsZero() {
			return codegenSourceTarget{}, "", false, newCodegenInternal(
				loc,
				"direct-choice named target has no generated component identifier",
				nil,
				errCodegenDirectChoicePlan,
			)
		}
		scalarKind, ok := codegenSourceScalarKindFromDigit(concrete.kind)
		if !ok {
			return codegenSourceTarget{}, "", false, newCodegenInternal(
				loc,
				"direct-choice source target has an unknown primitive kind",
				nil,
				errCodegenDirectChoicePlan,
			)
		}
		return codegenSourceTarget{
			form:         codegenSourceTargetNamed,
			declaredType: concrete.declaredType,
			typeID:       concrete.id,
			hasTypeID:    true,
			scalarKind:   scalarKind,
		}, concrete.componentIdentifier, false, nil
	default:
		return codegenSourceTarget{}, "", false, newCodegenInternal(
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
) (codegenSourceTarget, string, bool, error) {
	switch component.Kind() {
	case ComponentKindSimpleTypeDefinition:
		target, err := codegenNamedScalarTarget(schema, component, policyVersion)
		if err != nil {
			return codegenSourceTarget{}, "", false, err
		}
		fieldType, usesRuntime, err := codegenSourceTargetFieldType(target, names, runtimeAlias, hasRuntimeAlias, component.Loc())
		if err != nil {
			return codegenSourceTarget{}, "", false, err
		}
		return target, fieldType, usesRuntime, nil
	case ComponentKindElementDeclaration:
		return codegenElementFieldType(schema, names, component, runtimeAlias, hasRuntimeAlias, policyVersion, allowComplexElementType)
	case ComponentKindAttributeDeclaration,
		ComponentKindComplexTypeDefinition,
		ComponentKindModelGroupDefinition,
		ComponentKindAttributeGroupDefinition,
		ComponentKindNotationDeclaration:
		return codegenSourceTarget{}, "", false, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("schema component kind %q is not supported by Go scalar generation", component.Kind()),
			nil,
			fmt.Errorf("%w: component kind %q", errCodegenUnsupported, component.Kind()),
			policyVersion,
		)
	default:
		return codegenSourceTarget{}, "", false, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("unknown schema component kind %q is not supported by Go scalar generation", component.Kind()),
			nil,
			fmt.Errorf("%w: unknown component kind %q", errCodegenUnsupported, component.Kind()),
			policyVersion,
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

func codegenNamedScalarKind(component Component, version XSDVersion) (DigitDatatype, error) {
	definition, ok := component.SimpleTypeDefinition()
	if !ok {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has no supported simple-type view", component.Name()),
			nil,
			fmt.Errorf("%w: simple type view is missing", errCodegenUnsupported),
			version,
		)
	}
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has variety %q outside scalar Go generation", component.Name(), definition.Variety()),
			appendCodegenRelated(nil, definition.VarietyLoc()),
			fmt.Errorf("%w: simple type variety %q", errCodegenUnsupported, definition.Variety()),
			version,
		)
	}
	if definition.facts == nil ||
		schemaSimpleTypeAtomicKindIsUnsupported(definition.facts.atomicKind) ||
		definition.facts.atomicKind != schemaSimpleTypeAtomicInteger && definition.facts.atomicKind != schemaSimpleTypeAtomicDecimal {
		return "", newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has an unsupported atomic datatype", component.Name()),
			appendCodegenRelated(nil, definition.BaseLoc()),
			fmt.Errorf("%w: atomic datatype is outside scalar Go generation", errCodegenUnsupported),
			version,
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

func codegenNamedScalarTarget(schema Schema, component Component, version XSDVersion) (codegenSourceTarget, error) {
	definition, ok := component.SimpleTypeDefinition()
	if !ok {
		return codegenSourceTarget{}, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has no supported simple-type view", component.Name()),
			nil,
			fmt.Errorf("%w: simple type view is missing", errCodegenUnsupported),
			version,
		)
	}
	if definition.IsAnonymous() {
		return codegenSourceTarget{}, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("anonymous simple type %q is outside scalar Go generation", component.Name()),
			nil,
			fmt.Errorf("%w: anonymous simple type", errCodegenUnsupported),
			version,
		)
	}
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction {
		return codegenSourceTarget{}, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has variety %q outside scalar Go generation", component.Name(), definition.Variety()),
			appendCodegenRelated(nil, definition.VarietyLoc()),
			fmt.Errorf("%w: simple type variety %q", errCodegenUnsupported, definition.Variety()),
			version,
		)
	}
	if definition.facts == nil {
		return codegenSourceTarget{}, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("named simple type %q has no resolved primitive facts", component.Name()),
			appendCodegenRelated(nil, definition.BaseLoc()),
			errCodegenSchemaInvariant,
		)
	}
	if schemaSimpleTypeAtomicKindIsUnsupported(definition.facts.atomicKind) {
		return codegenSourceTarget{}, newCodegenUnsupported(
			component.Loc(),
			fmt.Sprintf("named simple type %q has an unsupported atomic datatype", component.Name()),
			appendCodegenRelated(nil, definition.BaseLoc()),
			fmt.Errorf("%w: atomic datatype is outside scalar Go generation", errCodegenUnsupported),
			version,
		)
	}
	if definition.facts.atomicKind == schemaSimpleTypeAtomicUnknown {
		if _, booleanFacets := definition.facts.facets.(schemaBooleanFacetVariant); !booleanFacets {
			return codegenSourceTarget{}, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("named simple type %q has inconsistent boolean primitive facts", component.Name()),
				appendCodegenRelated(nil, definition.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
	}
	if definition.IsBoolean() {
		if err := validateCodegenBooleanRestrictionChain(schema, component, definition, version); err != nil {
			return codegenSourceTarget{}, err
		}
		return codegenSourceTarget{
			form:         codegenSourceTargetDefinition,
			declaredType: component.Name(),
			typeID:       component.ID(),
			hasTypeID:    true,
			scalarKind:   codegenSourceScalarBoolean,
		}, nil
	}
	kind, err := codegenNamedScalarKind(component, version)
	if err != nil {
		return codegenSourceTarget{}, err
	}
	scalarKind, ok := codegenSourceScalarKindFromDigit(kind)
	if !ok {
		return codegenSourceTarget{}, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("named simple type %q has an unknown scalar kind", component.Name()),
			codegenSimpleTypeRelatedLocations(definition, definition.DigitFacets()),
			errCodegenSchemaInvariant,
		)
	}
	return codegenSourceTarget{
		form:         codegenSourceTargetDefinition,
		declaredType: component.Name(),
		typeID:       component.ID(),
		hasTypeID:    true,
		scalarKind:   scalarKind,
	}, nil
}

func codegenSourceScalarKindFromDigit(kind DigitDatatype) (codegenSourceScalarKind, bool) {
	switch kind {
	case DigitDatatypeInteger:
		return codegenSourceScalarInteger, true
	case DigitDatatypeDecimal:
		return codegenSourceScalarDecimal, true
	default:
		return codegenSourceScalarInvalid, false
	}
}

//nolint:gocognit,funlen // Keep the resolved boolean restriction-chain checks together.
func validateCodegenBooleanRestrictionChain(
	schema Schema,
	component Component,
	definition SimpleTypeDefinition,
	version XSDVersion,
) error {
	if component.ID().IsZero() {
		return newCodegenInternal(
			component.Loc(),
			"named boolean type has an empty component identity",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	seen := map[ComponentID]struct{}{component.ID(): {}}
	current := definition
	for {
		if current.IsAnonymous() {
			return newCodegenUnsupported(
				current.Loc(),
				"anonymous boolean restriction types are outside scalar Go generation",
				nil,
				fmt.Errorf("%w: anonymous boolean restriction type", errCodegenUnsupported),
				version,
			)
		}
		if current.facts == nil ||
			current.Variety() != SimpleTypeVarietyAtomicRestriction ||
			!current.IsBoolean() ||
			current.facts.atomicKind != schemaSimpleTypeAtomicUnknown {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction chain has inconsistent primitive facts",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		base, ok := current.BaseReference()
		if !ok || base.Kind() == "" {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction has no complete base reference",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		if base.Variety() != SimpleTypeVarietyAtomicRestriction || base.Name().IsZero() && !base.IsAnonymous() {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction base facts are incomplete",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		if base.facts == nil {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction base has no resolved primitive facts",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		if base.facts.atomicKind != schemaSimpleTypeAtomicUnknown {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction base has inconsistent primitive facts",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		if _, ok := base.facts.facets.(schemaBooleanFacetVariant); !ok {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction base has inconsistent primitive facts",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		if !base.IsAnonymous() && current.Base() != base.Name() {
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction base QName does not match its reference",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		baseID, hasBaseID := current.BaseID()
		if base.IsNamed() {
			referencedID, hasReferencedID := base.ComponentID()
			if !hasBaseID || baseID.IsZero() || !hasReferencedID || referencedID.IsZero() || baseID != referencedID {
				return newCodegenInternal(
					current.Loc(),
					"named boolean restriction base identity facts are inconsistent",
					appendCodegenRelated(nil, current.BaseLoc()),
					errCodegenSchemaInvariant,
				)
			}
		}
		if (base.IsBuiltin() || base.IsAnonymous()) && (hasBaseID || !baseID.IsZero()) {
			return newCodegenInternal(
				current.Loc(),
				"non-named boolean restriction base has a synthetic identity",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
		switch base.Kind() {
		case SimpleTypeReferenceBuiltin:
			if base.Name().Namespace() != xsdNamespaceURI || base.Name().Local() != "boolean" {
				return newCodegenInternal(
					current.Loc(),
					"built-in boolean restriction base does not identify xs:boolean",
					appendCodegenRelated(nil, current.BaseLoc()),
					errCodegenSchemaInvariant,
				)
			}
			return nil
		case SimpleTypeReferenceAnonymous:
			return newCodegenUnsupported(
				current.Loc(),
				"anonymous boolean restriction bases are outside scalar Go generation",
				appendCodegenRelated(nil, current.BaseLoc()),
				fmt.Errorf("%w: anonymous boolean restriction base", errCodegenUnsupported),
				version,
			)
		case SimpleTypeReferenceNamed:
			referencedID, hasReferencedID := base.ComponentID()
			if !hasReferencedID || referencedID.IsZero() {
				return newCodegenInternal(
					current.Loc(),
					"named boolean restriction base has no component identity",
					appendCodegenRelated(nil, current.BaseLoc()),
					errCodegenSchemaInvariant,
				)
			}
			if _, exists := seen[referencedID]; exists {
				return newCodegenInternal(
					current.Loc(),
					"named boolean restriction chain contains a cycle",
					appendCodegenRelated(nil, current.BaseLoc()),
					errCodegenSchemaInvariant,
				)
			}
			seen[referencedID] = struct{}{}
			target, found := schema.Lookup(referencedID)
			if !found || target.ID() != referencedID || target.Name() != base.Name() {
				return newCodegenInternal(
					current.Loc(),
					"named boolean restriction base identity does not match the schema",
					appendCodegenRelated(nil, target.Loc()),
					errCodegenSchemaInvariant,
				)
			}
			if target.Kind() != ComponentKindSimpleTypeDefinition {
				return newCodegenInternal(
					current.Loc(),
					"named boolean restriction base is not a simple type",
					appendCodegenRelated(nil, target.Loc()),
					errCodegenSchemaInvariant,
				)
			}
			baseDefinition, definitionOK := target.SimpleTypeDefinition()
			if !definitionOK {
				return newCodegenInternal(
					current.Loc(),
					"named boolean restriction base has no simple-type facts",
					appendCodegenRelated(nil, target.Loc()),
					errCodegenSchemaInvariant,
				)
			}
			current = baseDefinition
		default:
			return newCodegenInternal(
				current.Loc(),
				"named boolean restriction base has an unknown reference kind",
				appendCodegenRelated(nil, current.BaseLoc()),
				errCodegenSchemaInvariant,
			)
		}
	}
}

func codegenSourceTargetFieldType(
	target codegenSourceTarget,
	names codegenNaming,
	runtimeAlias string,
	hasRuntimeAlias bool,
	loc Loc,
) (string, bool, error) {
	switch target.form {
	case codegenSourceTargetDefinition, codegenSourceTargetBuiltin:
		if target.declaredType.IsZero() || target.hasTypeID && target.typeID.IsZero() {
			return "", false, newCodegenInternal(
				loc,
				"scalar source target has incomplete type identity",
				nil,
				errCodegenElementType,
			)
		}
		switch target.scalarKind {
		case codegenSourceScalarBoolean:
			return "bool", false, nil
		case codegenSourceScalarInteger:
			fieldType, err := codegenRuntimeScalarType(runtimeAlias, hasRuntimeAlias, DigitDatatypeInteger, loc)
			return fieldType, true, err
		case codegenSourceScalarDecimal:
			fieldType, err := codegenRuntimeScalarType(runtimeAlias, hasRuntimeAlias, DigitDatatypeDecimal, loc)
			return fieldType, true, err
		case codegenSourceScalarInvalid:
			return "", false, newCodegenInternal(
				loc,
				"scalar source target has an unknown primitive kind",
				nil,
				errCodegenSchemaInvariant,
			)
		default:
			return "", false, newCodegenInternal(
				loc,
				"scalar source target has an unknown primitive kind",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	case codegenSourceTargetNamed:
		if !target.hasTypeID || target.typeID.IsZero() || target.declaredType.IsZero() {
			return "", false, newCodegenInternal(
				loc,
				"named scalar source target has incomplete type identity",
				nil,
				errCodegenElementType,
			)
		}
		identifier, ok := names.componentName(target.typeID)
		if !ok || identifier == "" {
			return "", false, newCodegenNamingInvariant(
				loc,
				fmt.Sprintf("named scalar source target %s has no generated name", target.typeID.Source()),
				errCodegenNamingMisaligned,
			)
		}
		return identifier, false, nil
	case codegenSourceTargetInvalid:
		return "", false, newCodegenInternal(
			loc,
			"scalar source target has an unknown representation",
			nil,
			errCodegenSchemaInvariant,
		)
	default:
		return "", false, newCodegenInternal(
			loc,
			"scalar source target has an unknown representation",
			nil,
			errCodegenSchemaInvariant,
		)
	}
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
) (codegenSourceTarget, string, bool, error) {
	declaration, ok := component.ElementDeclaration()
	if !ok {
		return codegenSourceTarget{}, "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element %q has no supported declaration view", component.Name()),
			nil,
			fmt.Errorf("%w: element declaration view is missing", errCodegenUnsupported),
			version,
		)
	}
	declaredType := declaration.DeclaredType()
	if declaredType.IsZero() {
		return codegenSourceTarget{}, "", false, newCodegenElementUnsupported(
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
) (codegenSourceTarget, string, bool, error) {
	typeID, hasTypeID := declaration.TypeID()
	if hasTypeID || !typeID.IsZero() {
		return codegenSourceTarget{}, "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("built-in global element %q has a synthetic type identity", component.Name()),
			nil,
			errCodegenElementType,
		)
	}
	target := codegenSourceTarget{
		form:         codegenSourceTargetBuiltin,
		declaredType: declaration.DeclaredType(),
		scalarKind:   codegenSourceScalarInvalid,
	}
	switch declaration.DeclaredType().Local() {
	case "boolean":
		target.scalarKind = codegenSourceScalarBoolean
	case "integer":
		target.scalarKind = codegenSourceScalarInteger
	case "decimal":
		target.scalarKind = codegenSourceScalarDecimal
	case "language", "NCName", "anyURI", "ID":
		return codegenSourceTarget{}, "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element type %q is outside scalar Go generation", declaration.DeclaredType()),
			nil,
			fmt.Errorf("%w: built-in type %q", errCodegenUnsupported, declaration.DeclaredType()),
			version,
		)
	default:
		return codegenSourceTarget{}, "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element type %q is outside scalar Go generation", declaration.DeclaredType()),
			nil,
			fmt.Errorf("%w: built-in type %q", errCodegenUnsupported, declaration.DeclaredType()),
			version,
		)
	}
	if err := validateCodegenElementTypeReference(declaration, target, component.Loc()); err != nil {
		return codegenSourceTarget{}, "", false, err
	}
	if target.scalarKind == codegenSourceScalarBoolean {
		return target, "bool", false, nil
	}
	fieldType, _, err := codegenSourceTargetFieldType(target, codegenNaming{}, runtimeAlias, hasRuntimeAlias, component.Loc())
	if err != nil {
		return codegenSourceTarget{}, "", false, err
	}
	return target, fieldType, true, nil
}

func codegenNamedElementFieldType(
	schema Schema,
	names codegenNaming,
	component Component,
	declaration ElementDeclaration,
	version XSDVersion,
	allowComplexElementType bool,
) (codegenSourceTarget, string, bool, error) {
	declaredType := declaration.DeclaredType()
	typeID, hasTypeID := declaration.TypeID()
	if !hasTypeID || typeID.IsZero() {
		return codegenSourceTarget{}, "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("named global element %q has no resolved type identity", component.Name()),
			nil,
			errCodegenElementType,
		)
	}
	target, ok := schema.Lookup(typeID)
	if !ok {
		return codegenSourceTarget{}, "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("global element type identity %s is absent from the schema", typeID.Source()),
			nil,
			errCodegenElementType,
		)
	}
	related := appendCodegenRelated(nil, target.Loc())
	if target.ID() != typeID || target.Name() != declaredType {
		return codegenSourceTarget{}, "", false, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("global element %q type identity does not match its declared QName", component.Name()),
			related,
			errCodegenElementType,
		)
	}
	if target.Kind() == ComponentKindComplexTypeDefinition && allowComplexElementType {
		sourceTarget := codegenSourceTarget{
			form:         codegenSourceTargetNamed,
			declaredType: declaredType,
			typeID:       typeID,
			hasTypeID:    true,
		}
		fieldType, _, err := codegenSourceTargetFieldType(sourceTarget, names, "", false, component.Loc())
		if err != nil {
			return codegenSourceTarget{}, "", false, err
		}
		return sourceTarget, fieldType, false, nil
	}
	if target.Kind() != ComponentKindSimpleTypeDefinition {
		return codegenSourceTarget{}, "", false, newCodegenElementUnsupported(
			component.Loc(),
			fmt.Sprintf("global element type %q is not a supported named simple type", declaredType),
			related,
			fmt.Errorf("%w: target component kind %q", errCodegenUnsupported, target.Kind()),
			version,
		)
	}
	scalarTarget, err := codegenNamedScalarTarget(schema, target, version)
	if err != nil {
		return codegenSourceTarget{}, "", false, decorateCodegenElementError(err, component.Loc(), related)
	}
	sourceTarget := scalarTarget
	sourceTarget.form = codegenSourceTargetNamed
	sourceTarget.declaredType = declaredType
	sourceTarget.typeID = typeID
	sourceTarget.hasTypeID = true
	if referenceErr := validateCodegenElementTypeReference(declaration, sourceTarget, component.Loc()); referenceErr != nil {
		return codegenSourceTarget{}, "", false, decorateCodegenElementError(referenceErr, component.Loc(), related)
	}
	fieldType, _, err := codegenSourceTargetFieldType(sourceTarget, names, "", false, component.Loc())
	if err != nil {
		return codegenSourceTarget{}, "", false, err
	}
	return sourceTarget, fieldType, false, nil
}

//nolint:gocognit,funlen // Keep reference identity and primitive checks together.
func validateCodegenElementTypeReference(
	declaration ElementDeclaration,
	target codegenSourceTarget,
	loc Loc,
) error {
	reference, ok := declaration.TypeReference()
	if !ok || reference.facts == nil {
		return newCodegenInternal(
			loc,
			"global element type reference facts are missing",
			nil,
			errCodegenElementType,
		)
	}
	if reference.Name() != target.declaredType || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
		return newCodegenInternal(
			loc,
			"global element type reference does not match its declared type",
			nil,
			errCodegenElementType,
		)
	}
	switch target.form {
	case codegenSourceTargetBuiltin:
		if !reference.IsBuiltin() {
			return newCodegenInternal(
				loc,
				"built-in global element type reference is not built-in",
				nil,
				errCodegenElementType,
			)
		}
		if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
			return newCodegenInternal(
				loc,
				"built-in global element type reference has a component identity",
				nil,
				errCodegenElementType,
			)
		}
	case codegenSourceTargetNamed:
		if !reference.IsNamed() {
			return newCodegenInternal(
				loc,
				"named global element type reference is not named",
				nil,
				errCodegenElementType,
			)
		}
		referencedID, hasReferencedID := reference.ComponentID()
		if !hasReferencedID || referencedID != target.typeID {
			return newCodegenInternal(
				loc,
				"named global element type reference identity does not match its declaration",
				nil,
				errCodegenElementType,
			)
		}
	case codegenSourceTargetDefinition:
		return newCodegenInternal(
			loc,
			"global element type reference has an unresolved target form",
			nil,
			errCodegenElementType,
		)
	case codegenSourceTargetInvalid:
		return newCodegenInternal(
			loc,
			"global element type reference has an unknown target form",
			nil,
			errCodegenElementType,
		)
	default:
		return newCodegenInternal(
			loc,
			"global element type reference has an unknown target form",
			nil,
			errCodegenElementType,
		)
	}
	if schemaSimpleTypeAtomicKindIsUnsupported(reference.facts.atomicKind) {
		return newCodegenInternal(
			loc,
			"global element type reference has an unsupported atomic datatype",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	switch target.scalarKind {
	case codegenSourceScalarBoolean:
		if reference.facts.atomicKind != schemaSimpleTypeAtomicUnknown {
			return newCodegenInternal(
				loc,
				"global element boolean type reference has inconsistent primitive facts",
				nil,
				errCodegenSchemaInvariant,
			)
		}
		if _, ok := reference.facts.facets.(schemaBooleanFacetVariant); !ok {
			return newCodegenInternal(
				loc,
				"global element boolean type reference has inconsistent primitive facts",
				nil,
				errCodegenSchemaInvariant,
			)
		}
		if target.form == codegenSourceTargetBuiltin &&
			(reference.Name().Namespace() != xsdNamespaceURI || reference.Name().Local() != "boolean") {
			return newCodegenInternal(
				loc,
				"built-in global element boolean type reference does not identify xs:boolean",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	case codegenSourceScalarInteger:
		if reference.facts.atomicKind != schemaSimpleTypeAtomicInteger {
			return newCodegenInternal(
				loc,
				"global element integer type reference has inconsistent primitive facts",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	case codegenSourceScalarDecimal:
		if reference.facts.atomicKind != schemaSimpleTypeAtomicDecimal {
			return newCodegenInternal(
				loc,
				"global element decimal type reference has inconsistent primitive facts",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	case codegenSourceScalarInvalid:
		return newCodegenInternal(
			loc,
			"global element type reference has an unknown primitive kind",
			nil,
			errCodegenSchemaInvariant,
		)
	default:
		return newCodegenInternal(
			loc,
			"global element type reference has an unknown primitive kind",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	return nil
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

//nolint:gocognit // Keep source validation and ordered declaration rendering together.
func renderCodegenSource(plan codegenSourcePlan, schemas ...Schema) ([]byte, error) {
	if len(schemas) > 1 {
		return nil, newCodegenInternal(
			codegenSourcePlanLoc(plan),
			"render scalar source received multiple schemas",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	if len(schemas) == 1 {
		if err := validateCodegenSourcePlan(schemas[0], plan); err != nil {
			return nil, err
		}
	}
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
		if declaration.sequence != nil {
			if declaration.choice != nil {
				return nil, newCodegenInternal(
					declaration.loc,
					"render declaration has both sequence and choice facts",
					nil,
					errCodegenSchemaInvariant,
				)
			}
			if err := renderCodegenSequenceDeclaration(&source, declaration); err != nil {
				return nil, err
			}
			continue
		}
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

//nolint:gocognit,funlen // Keep render-boundary recollection and comparison explicit.
func validateCodegenSourcePlan(schema Schema, plan codegenSourcePlan) error {
	planLoc := codegenSourcePlanLoc(plan)
	if err := validateCodegenPackageName(plan.packageName); err != nil {
		return newCodegenInternal(planLoc, "source plan package name is invalid", nil, err)
	}
	if plan.names.packageIdentifier() != plan.packageName {
		return newCodegenInternal(
			planLoc,
			"source plan naming package does not match its package name",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	if schema.storage == nil {
		return newDiagnostic(
			FailureInvalid,
			diagnosticCodegenSchemaInvalid,
			Loc{},
			"code-generation schema is zero or incomplete",
			errCodegenSchemaEmpty,
		)
	}
	components := schema.Components()
	if err := validateCodegenSchemaStorage(schema, components); err != nil {
		return err
	}
	if _, _, _, err := validateCodegenInput(schema, plan.names); err != nil {
		return newCodegenInternal(planLoc, "source plan naming state is invalid", nil, err)
	}
	if err := validateCodegenScopedNamingIndexes(plan.names, planLoc); err != nil {
		return err
	}
	modes, modeErr := collectCodegenSourceSchemaModes(components)
	if modeErr != nil {
		return modeErr
	}
	if err := validateCodegenSourcePlanModes(plan, modes, planLoc); err != nil {
		return err
	}
	if !plan.directChoices && !plan.directSequences && (len(plan.names.fields) != 0 || len(plan.names.variants) != 0) {
		return newCodegenInternal(
			planLoc,
			"scalar source plan has scoped naming records without direct particles",
			nil,
			errCodegenSchemaInvariant,
		)
	}

	var expected codegenSourcePlan
	var err error
	if plan.directParticles {
		directPlan, directErr := planCodegenDirectParticles(schema, plan.packageName)
		if directErr != nil {
			return directErr
		}
		expected, err = planCodegenSourceWithDirectParticles(schema, directPlan)
	}
	if !plan.directParticles && plan.directChoices {
		choicePlan, choiceErr := planCodegenDirectChoices(schema, plan.packageName)
		if choiceErr != nil {
			return choiceErr
		}
		expected, err = planCodegenSourceWithDirectChoices(schema, choicePlan)
	}
	if !plan.directParticles && !plan.directChoices {
		for _, component := range components {
			if component.Kind() != ComponentKindComplexTypeDefinition {
				continue
			}
			return newCodegenInternal(
				component.Loc(),
				"scalar source plan does not include the schema direct-choice phase",
				nil,
				errCodegenSchemaInvariant,
			)
		}
		expectedNames, namingErr := newCodegenNaming(codegenNamingInput{
			packageName:   plan.packageName,
			schema:        schema,
			importAliases: codegenImportAliasRequests(plan.names),
		})
		if namingErr != nil {
			return newCodegenInternal(
				planLoc,
				"source plan naming state could not be recollected",
				nil,
				namingErr,
			)
		}
		expected, err = planCodegenSource(schema, expectedNames)
	}
	if err != nil {
		return err
	}
	if err := compareCodegenNaming(plan.names, expected.names, planLoc); err != nil {
		return err
	}
	return compareCodegenSourcePlans(plan, expected)
}

type codegenSourceSchemaModes struct {
	directChoices   bool
	directSequences bool
	choiceLoc       Loc
	sequenceLoc     Loc
}

//nolint:gocognit // Keep ordered schema mode recollection explicit.
func collectCodegenSourceSchemaModes(components []Component) (codegenSourceSchemaModes, error) {
	var modes codegenSourceSchemaModes
	for _, component := range components {
		if component.Kind() != ComponentKindComplexTypeDefinition {
			continue
		}
		definition, ok := component.ComplexType()
		if !ok {
			return codegenSourceSchemaModes{}, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("complex type %q has no completed complex-type facts", component.Name()),
				nil,
				errCodegenSchemaInvariant,
			)
		}
		particle := definition.Particle()
		if particle == nil {
			continue
		}
		if directChoiceTypedNilParticle(particle) {
			return codegenSourceSchemaModes{}, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("complex type %q has a typed-nil direct particle", component.Name()),
				nil,
				errCodegenSchemaInvariant,
			)
		}
		if choice, ok := directChoiceValue(particle); ok {
			modes.directChoices = true
			if modes.choiceLoc.IsZero() {
				modes.choiceLoc = choice.Loc()
			}
			continue
		}
		if sequence, ok := directSequenceValue(particle); ok {
			modes.directSequences = true
			if modes.sequenceLoc.IsZero() {
				modes.sequenceLoc = sequence.Loc()
			}
		}
	}
	return modes, nil
}

func validateCodegenSourcePlanModes(plan codegenSourcePlan, modes codegenSourceSchemaModes, planLoc Loc) error {
	if plan.directChoices != modes.directChoices {
		loc := planLoc
		if modes.directChoices {
			loc = modes.choiceLoc
		}
		if modes.directSequences {
			loc = modes.sequenceLoc
		}
		return newCodegenInternal(loc, "source plan direct-choice mode does not match the schema", nil, errCodegenSchemaInvariant)
	}
	if plan.directSequences != modes.directSequences {
		loc := planLoc
		if modes.directSequences {
			loc = modes.sequenceLoc
		}
		return newCodegenInternal(loc, "source plan direct-sequence mode does not match the schema", nil, errCodegenSchemaInvariant)
	}
	if plan.directSequences && !plan.directParticles {
		return newCodegenInternal(
			planLoc,
			"source plan direct-sequence mode requires the direct-particle phase",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	return nil
}

func codegenImportAliasRequests(names codegenNaming) []codegenImportAliasRequest {
	requests := make([]codegenImportAliasRequest, 0, len(names.imports))
	for _, imported := range names.imports {
		requests = append(requests, codegenImportAliasRequest{
			identity: imported.identity,
			alias:    imported.alias,
		})
	}
	return requests
}

func validateCodegenScopedNamingIndexes(names codegenNaming, loc Loc) error {
	if len(names.fieldByKey) != len(names.fields) {
		return newCodegenInternal(
			loc,
			"source plan field naming index has the wrong size",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	for _, field := range names.fields {
		identifier, ok := names.fieldName(field.owner, field.path)
		if !ok || identifier != field.identifier {
			return newCodegenInternal(
				loc,
				"source plan field naming index is stale",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	}
	if len(names.variantByKey) != len(names.variants) {
		return newCodegenInternal(
			loc,
			"source plan variant naming index has the wrong size",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	for _, variant := range names.variants {
		identifier, ok := names.variantName(variant.owner, variant.path)
		if !ok || identifier != variant.identifier {
			return newCodegenInternal(
				loc,
				"source plan variant naming index is stale",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	}
	return nil
}

//nolint:gocognit // Keep complete ordered source-plan comparison together.
func compareCodegenSourcePlans(actual, expected codegenSourcePlan) error {
	loc := codegenSourcePlanLoc(expected)
	if actual.packageName != expected.packageName {
		return newCodegenInternal(loc, "source plan package names do not match", nil, errCodegenSchemaInvariant)
	}
	if actual.runtimeAlias != expected.runtimeAlias {
		return newCodegenInternal(loc, "source plan runtime aliases do not match", nil, errCodegenSchemaInvariant)
	}
	if actual.useRuntime != expected.useRuntime {
		return newCodegenInternal(loc, "source plan runtime requirements do not match", nil, errCodegenSchemaInvariant)
	}
	if actual.directChoices != expected.directChoices {
		return newCodegenInternal(loc, "source plan direct-choice mode does not match", nil, errCodegenSchemaInvariant)
	}
	if actual.directSequences != expected.directSequences {
		return newCodegenInternal(loc, "source plan direct-sequence mode does not match", nil, errCodegenSchemaInvariant)
	}
	if actual.directParticles != expected.directParticles {
		return newCodegenInternal(loc, "source plan direct-particle mode does not match", nil, errCodegenSchemaInvariant)
	}
	if len(actual.declarations) != len(expected.declarations) {
		return newCodegenInternal(loc, "source plan declaration count does not match schema order", nil, errCodegenSchemaInvariant)
	}
	for index, expectedDeclaration := range expected.declarations {
		actualDeclaration := actual.declarations[index]
		declarationLoc := expectedDeclaration.loc
		if declarationLoc.IsZero() {
			declarationLoc = actualDeclaration.loc
		}
		if actualDeclaration.id != expectedDeclaration.id ||
			actualDeclaration.schemaName != expectedDeclaration.schemaName ||
			actualDeclaration.loc != expectedDeclaration.loc ||
			actualDeclaration.name != expectedDeclaration.name ||
			actualDeclaration.fieldType != expectedDeclaration.fieldType ||
			actualDeclaration.usesRuntime != expectedDeclaration.usesRuntime ||
			actualDeclaration.target != expectedDeclaration.target {
			return newCodegenInternal(
				declarationLoc,
				"source plan declaration facts do not match schema order",
				nil,
				errCodegenSchemaInvariant,
			)
		}
		if err := compareCodegenSourceChoices(actualDeclaration.choice, expectedDeclaration.choice, declarationLoc); err != nil {
			return err
		}
		if err := compareCodegenSourceSequences(actualDeclaration.sequence, expectedDeclaration.sequence, declarationLoc); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocognit // Keep ordered naming-record comparison explicit.
func compareCodegenNaming(actual, expected codegenNaming, loc Loc) error {
	if actual.packageIdentifier() != expected.packageIdentifier() {
		return newCodegenInternal(loc, "source plan naming packages do not match", nil, errCodegenSchemaInvariant)
	}
	if len(actual.components) != len(expected.components) {
		return newCodegenInternal(loc, "source plan component naming records do not match", nil, errCodegenSchemaInvariant)
	}
	for index, expectedComponent := range expected.components {
		actualComponent := actual.components[index]
		if actualComponent != expectedComponent {
			return newCodegenInternal(loc, "source plan component naming record does not match", nil, errCodegenSchemaInvariant)
		}
	}
	if len(actual.fields) != len(expected.fields) {
		return newCodegenInternal(loc, "source plan field naming records do not match", nil, errCodegenSchemaInvariant)
	}
	for index, expectedField := range expected.fields {
		actualField := actual.fields[index]
		if actualField.owner != expectedField.owner ||
			!equalCodegenPath(actualField.path, expectedField.path) ||
			actualField.name != expectedField.name ||
			actualField.identifier != expectedField.identifier {
			return newCodegenInternal(loc, "source plan field naming record does not match", nil, errCodegenSchemaInvariant)
		}
	}
	if len(actual.variants) != len(expected.variants) {
		return newCodegenInternal(loc, "source plan variant naming records do not match", nil, errCodegenSchemaInvariant)
	}
	for index, expectedVariant := range expected.variants {
		actualVariant := actual.variants[index]
		if actualVariant.owner != expectedVariant.owner ||
			!equalCodegenPath(actualVariant.path, expectedVariant.path) ||
			actualVariant.name != expectedVariant.name ||
			actualVariant.identifier != expectedVariant.identifier {
			return newCodegenInternal(loc, "source plan variant naming record does not match", nil, errCodegenSchemaInvariant)
		}
	}
	if len(actual.imports) != len(expected.imports) {
		return newCodegenInternal(loc, "source plan import naming records do not match", nil, errCodegenSchemaInvariant)
	}
	for index, expectedImport := range expected.imports {
		if actual.imports[index] != expectedImport {
			return newCodegenInternal(loc, "source plan import naming record does not match", nil, errCodegenSchemaInvariant)
		}
	}
	return nil
}

func compareCodegenSourceChoices(actual, expected *codegenSourceChoice, loc Loc) error {
	if actual == nil || expected == nil {
		if actual == nil && expected == nil {
			return nil
		}
		return newCodegenInternal(loc, "source plan choice presence does not match schema", nil, errCodegenSchemaInvariant)
	}
	if actual.ownerID != expected.ownerID ||
		actual.ownerName != expected.ownerName ||
		actual.ownerLoc != expected.ownerLoc ||
		actual.choiceLoc != expected.choiceLoc ||
		actual.marker != expected.marker ||
		actual.usesRuntime != expected.usesRuntime ||
		len(actual.variants) != len(expected.variants) {
		return newCodegenInternal(loc, "source plan choice facts do not match schema", nil, errCodegenSchemaInvariant)
	}
	for index, expectedVariant := range expected.variants {
		actualVariant := actual.variants[index]
		if !equalCodegenPath(actualVariant.path, expectedVariant.path) ||
			actualVariant.loc != expectedVariant.loc ||
			actualVariant.schemaName != expectedVariant.schemaName ||
			actualVariant.name != expectedVariant.name ||
			actualVariant.fieldName != expectedVariant.fieldName ||
			actualVariant.fieldType != expectedVariant.fieldType ||
			actualVariant.usesRuntime != expectedVariant.usesRuntime ||
			actualVariant.target != expectedVariant.target {
			return newCodegenInternal(
				actualVariant.loc,
				"source plan choice alternative facts do not match schema",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	}
	return nil
}

func compareCodegenSourceSequences(actual, expected *codegenSourceSequence, loc Loc) error {
	if actual == nil || expected == nil {
		if actual == nil && expected == nil {
			return nil
		}
		return newCodegenInternal(loc, "source plan sequence presence does not match schema", nil, errCodegenSchemaInvariant)
	}
	if actual.ownerID != expected.ownerID ||
		actual.ownerName != expected.ownerName ||
		actual.ownerLoc != expected.ownerLoc ||
		actual.sequenceLoc != expected.sequenceLoc ||
		actual.usesRuntime != expected.usesRuntime ||
		len(actual.fields) != len(expected.fields) {
		return newCodegenInternal(loc, "source plan sequence facts do not match schema", nil, errCodegenSchemaInvariant)
	}
	for index, expectedField := range expected.fields {
		actualField := actual.fields[index]
		if !equalCodegenPath(actualField.path, expectedField.path) ||
			actualField.loc != expectedField.loc ||
			actualField.schemaName != expectedField.schemaName ||
			actualField.fieldName != expectedField.fieldName ||
			actualField.fieldType != expectedField.fieldType ||
			actualField.usesRuntime != expectedField.usesRuntime ||
			actualField.target != expectedField.target {
			return newCodegenInternal(
				codegenSourceSequenceFieldLoc(expectedField, expected.sequenceLoc, loc),
				"source plan sequence field facts do not match schema",
				nil,
				errCodegenSchemaInvariant,
			)
		}
	}
	return nil
}

func codegenSourceSequenceFieldLoc(field codegenSourceSequenceField, sequenceLoc, declarationLoc Loc) Loc {
	if !field.loc.IsZero() {
		return field.loc
	}
	if !sequenceLoc.IsZero() {
		return sequenceLoc
	}
	return declarationLoc
}

func codegenSourcePlanLoc(plan codegenSourcePlan) Loc {
	for _, declaration := range plan.declarations {
		if !declaration.loc.IsZero() {
			return declaration.loc
		}
		if declaration.choice != nil && !declaration.choice.ownerLoc.IsZero() {
			return declaration.choice.ownerLoc
		}
		if declaration.sequence != nil && !declaration.sequence.ownerLoc.IsZero() {
			return declaration.sequence.ownerLoc
		}
	}
	return Loc{}
}

func renderCodegenChoiceDeclaration(source *strings.Builder, declaration codegenSourceDeclaration) error {
	choice := declaration.choice
	if choice == nil {
		return newCodegenInternal(
			declaration.loc,
			"render direct-choice declaration without choice facts",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	if !validCodegenDirectChoiceMarker(choice.marker) {
		return newCodegenInternal(
			declaration.loc,
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

func renderCodegenSequenceDeclaration(source *strings.Builder, declaration codegenSourceDeclaration) error {
	sequence := declaration.sequence
	if sequence == nil {
		return newCodegenInternal(
			declaration.loc,
			"render direct-sequence declaration without sequence facts",
			nil,
			errCodegenSchemaInvariant,
		)
	}
	source.WriteString("type ")
	source.WriteString(declaration.name)
	source.WriteString(" struct {")
	if len(sequence.fields) == 0 {
		source.WriteString("}\n\n")
		return nil
	}
	source.WriteByte('\n')
	for _, field := range sequence.fields {
		if field.fieldName == "" || field.fieldType == "" {
			return newCodegenInternal(
				field.loc,
				"render direct-sequence declaration with incomplete field facts",
				nil,
				errCodegenDirectSequencePlan,
			)
		}
		source.WriteString("\t")
		source.WriteString(field.fieldName)
		source.WriteByte(' ')
		source.WriteString(field.fieldType)
		source.WriteByte('\n')
	}
	source.WriteString("}\n\n")
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
