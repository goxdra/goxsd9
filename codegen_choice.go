package goxsd9

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	codegenDirectChoiceXSD10ParticlesSpecRef       = "xsd10-structures#cParticles"
	codegenDirectChoiceXSD10ElementChoiceSpecRef   = "xsd10-structures#element-choice"
	codegenDirectChoiceXSD10ParticleDetailsSpecRef = "xsd10-structures#Particle_details"
	codegenDirectChoiceXSD11ParticlesSpecRef       = "xsd11-structures#cParticles"
	codegenDirectChoiceXSD11ElementChoiceSpecRef   = "xsd11-structures#element-choice"
	codegenDirectChoiceXSD11ParticleDetailsSpecRef = "xsd11-structures#Particle_details"
)

var (
	errCodegenDirectChoiceQName    = errors.New("direct choice QName is malformed")
	errCodegenDirectChoiceParticle = errors.New("direct choice particle fact is incomplete")
	errCodegenDirectChoiceTarget   = errors.New("direct choice scalar target fact is incomplete")
	errCodegenDirectChoiceResolve  = errors.New("direct choice scalar target could not be resolved")
	errCodegenDirectChoiceNaming   = errors.New("direct choice naming table is misaligned")
	errCodegenDirectChoicePlan     = errors.New("direct choice plan invariant is broken")
)

// codegenDirectChoicePlan is the private, source-free plan for modeled direct
// scalar choices. Ordered owners and alternatives are the source of truth.
type codegenDirectChoicePlan struct {
	packageName  string
	runtimeAlias string
	names        codegenNaming
	owners       []codegenDirectChoiceOwner
}

type codegenDirectChoiceOwner struct {
	id           ComponentID
	name         QName
	loc          Loc
	identifier   string
	marker       string
	choiceLoc    Loc
	alternatives []codegenDirectChoiceAlternative
}

type codegenDirectChoiceAlternative struct {
	path              []uint32
	loc               Loc
	name              QName
	fieldIdentifier   string
	variantIdentifier string
	target            codegenDirectChoiceTarget
}

// codegenDirectChoiceTarget has concrete built-in and named scalar forms.
// Built-in targets never carry a synthetic ComponentID.
type codegenDirectChoiceTarget interface {
	codegenDirectChoiceTarget()
}

type codegenDirectChoiceBuiltinTarget struct {
	declaredType QName
	kind         DigitDatatype
}

func (codegenDirectChoiceBuiltinTarget) codegenDirectChoiceTarget() {}

type codegenDirectChoiceNamedTarget struct {
	declaredType        QName
	id                  ComponentID
	kind                DigitDatatype
	componentIdentifier string
}

func (codegenDirectChoiceNamedTarget) codegenDirectChoiceTarget() {}

type codegenDirectChoiceCollectedOwner struct {
	id           ComponentID
	name         QName
	loc          Loc
	choiceLoc    Loc
	alternatives []codegenDirectChoiceCollectedAlternative
}

type codegenDirectChoiceCollectedAlternative struct {
	path   []uint32
	loc    Loc
	name   QName
	target codegenDirectChoiceTarget
}

// planCodegenDirectChoices collects, validates, names, and materializes one
// complete private direct-choice plan. It never returns a partial plan.
func planCodegenDirectChoices(schema Schema, packageName string) (codegenDirectChoicePlan, error) {
	if err := validateCodegenPackageName(packageName); err != nil {
		return codegenDirectChoicePlan{}, err
	}
	if schema.storage == nil {
		return codegenDirectChoicePlan{}, newDiagnostic(
			FailureInvalid,
			diagnosticCodegenSchemaInvalid,
			Loc{},
			"direct-choice code-generation schema is zero or incomplete",
			errCodegenSchemaEmpty,
		)
	}

	components := schema.Components()
	if err := validateCodegenSchemaStorage(schema, components); err != nil {
		return codegenDirectChoicePlan{}, err
	}
	version, err := codegenSchemaVersion(schema)
	if err != nil {
		return codegenDirectChoicePlan{}, err
	}
	collected, err := collectCodegenDirectChoices(schema, components, version)
	if err != nil {
		return codegenDirectChoicePlan{}, err
	}

	input := codegenDirectChoiceNamingInput(packageName, schema, collected)
	names, err := newCodegenNaming(input)
	if err != nil {
		return codegenDirectChoicePlan{}, err
	}
	if namingErr := validateCodegenDirectChoiceNaming(components, collected, names); namingErr != nil {
		return codegenDirectChoicePlan{}, namingErr
	}
	plan, err := materializeCodegenDirectChoicePlan(packageName, collected, names)
	if err != nil {
		return codegenDirectChoicePlan{}, err
	}
	if err := validateCodegenDirectChoicePlan(schema, plan); err != nil {
		return codegenDirectChoicePlan{}, err
	}
	return plan, nil
}

//nolint:gocognit,funlen // Keep collection and ordered particle preflight in one phase.
func collectCodegenDirectChoices(
	schema Schema,
	components []Component,
	version XSDVersion,
) ([]codegenDirectChoiceCollectedOwner, error) {
	owners := make([]codegenDirectChoiceCollectedOwner, 0)
	for _, component := range components {
		if component.Kind() != ComponentKindComplexTypeDefinition {
			continue
		}
		definition, ok := component.ComplexType()
		if !ok {
			return nil, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("complex type %q has no completed complex-type facts", component.Name()),
				nil,
				errCodegenDirectChoiceParticle,
			)
		}
		particle := definition.Particle()
		if particle == nil {
			return nil, newCodegenDirectChoiceUnsupported(
				component.Loc(),
				fmt.Sprintf("complex type %q has no direct choice particle", component.Name()),
				nil,
				fmt.Errorf("%w: complex type has no modeled particle", errCodegenUnsupported),
				version,
				codegenDirectChoiceParticlesReference,
			)
		}
		if directChoiceTypedNilParticle(particle) {
			return nil, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("complex type %q has a typed-nil choice particle", component.Name()),
				nil,
				errCodegenDirectChoiceParticle,
			)
		}

		choice, choiceOK := directChoiceValue(particle)
		if !choiceOK {
			if nested, nestedOK := directChoiceNestedChoice(particle); nestedOK && nested.facts == nil {
				return nil, newCodegenInternal(
					component.Loc(),
					fmt.Sprintf("complex type %q has an incomplete choice particle", component.Name()),
					nil,
					errCodegenDirectChoiceParticle,
				)
			}
			return nil, newCodegenDirectChoiceUnsupported(
				component.Loc(),
				fmt.Sprintf("complex type %q particle is outside direct choice generation", component.Name()),
				nil,
				fmt.Errorf("%w: complex type particle is not a choice", errCodegenUnsupported),
				version,
				codegenDirectChoiceParticlesReference,
			)
		}
		if choice.facts == nil {
			return nil, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("complex type %q has an incomplete choice particle", component.Name()),
				nil,
				errCodegenDirectChoiceParticle,
			)
		}
		if err := validateCodegenDirectChoiceBounds(
			choice.MinOccurs(),
			choice.MaxOccurs(),
			choice.Loc(),
			"choice",
			version,
		); err != nil {
			return nil, err
		}

		owner := codegenDirectChoiceCollectedOwner{
			id:           component.ID(),
			name:         component.Name(),
			loc:          component.Loc(),
			choiceLoc:    choice.Loc(),
			alternatives: make([]codegenDirectChoiceCollectedAlternative, 0, len(choice.Alternatives())),
		}
		alternatives := choice.Alternatives()
		for index, alternative := range alternatives {
			if alternative == nil {
				return nil, newCodegenInternal(
					choice.Loc(),
					"direct-choice alternative is nil",
					nil,
					errCodegenDirectChoiceParticle,
				)
			}
			if directChoiceTypedNilParticle(alternative) {
				return nil, newCodegenInternal(
					choice.Loc(),
					"direct-choice alternative is a typed-nil particle",
					nil,
					errCodegenDirectChoiceParticle,
				)
			}
			path, pathErr := codegenDirectChoicePath(index)
			if pathErr != nil {
				return nil, newCodegenInternal(
					choice.Loc(),
					"construct direct-choice alternative path",
					nil,
					pathErr,
				)
			}
			element, elementOK := directChoiceValueElement(alternative)
			if !elementOK {
				if nested, nestedOK := directChoiceNestedChoice(alternative); nestedOK && nested.facts == nil {
					return nil, newCodegenInternal(
						choice.Loc(),
						"direct-choice alternative contains an incomplete nested choice",
						nil,
						errCodegenDirectChoiceParticle,
					)
				}
				return nil, newCodegenDirectChoiceUnsupported(
					choice.Loc(),
					"nested or non-element choice alternatives are outside direct choice generation",
					nil,
					fmt.Errorf("%w: choice alternative is not a direct element", errCodegenUnsupported),
					version,
					codegenDirectChoiceParticlesReference,
				)
			}
			if element.facts == nil {
				return nil, newCodegenInternal(
					choice.Loc(),
					"direct-choice element alternative has incomplete particle facts",
					nil,
					errCodegenDirectChoiceParticle,
				)
			}
			if err := validateCodegenDirectChoiceBounds(
				element.MinOccurs(),
				element.MaxOccurs(),
				element.Loc(),
				"choice element",
				version,
			); err != nil {
				return nil, err
			}
			if err := validateCodegenDirectChoiceQName(element.Name(), element.Loc(), "local element"); err != nil {
				return nil, err
			}
			target, targetErr := validateCodegenDirectChoiceTarget(schema, element, version)
			if targetErr != nil {
				return nil, targetErr
			}
			owner.alternatives = append(owner.alternatives, codegenDirectChoiceCollectedAlternative{
				path:   cloneCodegenPath(path),
				loc:    element.Loc(),
				name:   element.Name(),
				target: target,
			})
		}
		owners = append(owners, owner)
	}
	return owners, nil
}

func directChoiceTypedNilParticle(particle Particle) bool {
	switch concrete := particle.(type) {
	case *ChoiceParticle:
		return concrete == nil
	case *ElementParticle:
		return concrete == nil
	default:
		return false
	}
}

func directChoiceValue(particle Particle) (ChoiceParticle, bool) {
	switch concrete := particle.(type) {
	case ChoiceParticle:
		return concrete, true
	case *ChoiceParticle:
		if concrete == nil {
			return ChoiceParticle{}, false
		}
		return *concrete, true
	default:
		return ChoiceParticle{}, false
	}
}

func directChoiceValueElement(particle Particle) (ElementParticle, bool) {
	switch concrete := particle.(type) {
	case ElementParticle:
		return concrete, true
	case *ElementParticle:
		if concrete == nil {
			return ElementParticle{}, false
		}
		return *concrete, true
	default:
		return ElementParticle{}, false
	}
}

func directChoiceNestedChoice(particle Particle) (ChoiceParticle, bool) {
	return directChoiceValue(particle)
}

func codegenDirectChoicePath(index int) ([]uint32, error) {
	if index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return nil, fmt.Errorf("%w: alternative ordinal overflows uint32", errCodegenDirectChoicePlan)
	}
	return []uint32{1, uint32(index + 1)}, nil
}

func validateCodegenDirectChoiceBounds(
	minimum, maximum uint64,
	loc Loc,
	context string,
	version XSDVersion,
) error {
	if minimum > maximum {
		return newCodegenInternal(
			loc,
			context+" occurrence bounds are inconsistent",
			nil,
			errCodegenDirectChoiceParticle,
		)
	}
	if minimum == 1 && maximum == 1 {
		return nil
	}
	return newCodegenDirectChoiceUnsupported(
		loc,
		fmt.Sprintf("%s occurrence bounds %d/%d are outside direct choice generation", context, minimum, maximum),
		nil,
		fmt.Errorf("%w: non-default %s occurrence bounds", errCodegenUnsupported, context),
		version,
		codegenDirectChoiceParticleDetailsReference,
	)
}

func validateCodegenDirectChoiceQName(name QName, loc Loc, context string) error {
	if !utf8.ValidString(name.Namespace()) || !utf8.ValidString(name.Local()) {
		return newCodegenSchemaInvalid(
			loc,
			context+" QName is not valid UTF-8",
			errCodegenDirectChoiceQName,
		)
	}
	if name.Local() == "" {
		return newCodegenSchemaInvalid(
			loc,
			context+" QName has an empty local name",
			errCodegenDirectChoiceQName,
		)
	}
	return nil
}

//nolint:gocognit,funlen // Keep scalar target identity and classification checks together.
func validateCodegenDirectChoiceTarget(
	schema Schema,
	element ElementParticle,
	version XSDVersion,
) (codegenDirectChoiceTarget, error) {
	declaredType := element.DeclaredType()
	typeID, hasTypeID := element.TypeID()
	if declaredType.IsZero() {
		if hasTypeID || !typeID.IsZero() {
			return nil, newCodegenInternal(
				element.Loc(),
				"anonymous direct-choice type has a synthetic component identity",
				nil,
				errCodegenDirectChoiceTarget,
			)
		}
		return nil, newCodegenDirectChoiceUnsupported(
			element.Loc(),
			"anonymous direct-choice element types are outside direct choice generation",
			nil,
			fmt.Errorf("%w: anonymous element type", errCodegenUnsupported),
			version,
			codegenDirectChoiceElementChoiceReference,
		)
	}
	if err := validateCodegenDirectChoiceQName(declaredType, element.Loc(), "declared type"); err != nil {
		return nil, err
	}
	if declaredType.Namespace() == xsdNamespaceURI {
		if hasTypeID || !typeID.IsZero() {
			return nil, newCodegenInternal(
				element.Loc(),
				fmt.Sprintf("built-in direct-choice type %q has a synthetic component identity", declaredType),
				nil,
				errCodegenDirectChoiceTarget,
			)
		}
		switch declaredType.Local() {
		case "integer":
			return codegenDirectChoiceBuiltinTarget{declaredType: declaredType, kind: DigitDatatypeInteger}, nil
		case "decimal":
			return codegenDirectChoiceBuiltinTarget{declaredType: declaredType, kind: DigitDatatypeDecimal}, nil
		default:
			return nil, newCodegenDirectChoiceUnsupported(
				element.Loc(),
				fmt.Sprintf("built-in direct-choice type %q is outside scalar generation", declaredType),
				nil,
				fmt.Errorf("%w: built-in scalar type %q", errCodegenUnsupported, declaredType),
				version,
				codegenDirectChoiceElementChoiceReference,
			)
		}
	}
	if !hasTypeID {
		return nil, newCodegenDirectChoiceResolution(
			element.Loc(),
			fmt.Sprintf("named direct-choice type %q has no resolved component identity", declaredType),
			nil,
			errCodegenDirectChoiceResolve,
		)
	}
	if typeID.Source() == "" || typeID.Ordinal() == 0 {
		return nil, newCodegenInternal(
			element.Loc(),
			fmt.Sprintf("named direct-choice type %q has an invalid component identity", declaredType),
			nil,
			errCodegenDirectChoiceTarget,
		)
	}
	target, ok := schema.Lookup(typeID)
	if !ok {
		return nil, newCodegenDirectChoiceResolution(
			element.Loc(),
			fmt.Sprintf("named direct-choice type identity %s is absent from the schema", typeID.Source()),
			nil,
			errCodegenDirectChoiceResolve,
		)
	}
	related := appendCodegenRelated(nil, target.Loc())
	if target.ID() != typeID || target.Name() != declaredType {
		return nil, newCodegenInternal(
			element.Loc(),
			fmt.Sprintf("named direct-choice type identity does not match declared QName %q", declaredType),
			related,
			errCodegenDirectChoiceTarget,
		)
	}
	if target.Kind() != ComponentKindSimpleTypeDefinition {
		return nil, newCodegenInternal(
			element.Loc(),
			fmt.Sprintf("named direct-choice target %q has kind %q, not a simple type", declaredType, target.Kind()),
			related,
			errCodegenDirectChoiceTarget,
		)
	}
	if err := validateCodegenDirectChoiceQName(target.Name(), target.Loc(), "named scalar target"); err != nil {
		return nil, err
	}
	if _, ok := target.SimpleTypeDefinition(); !ok {
		return nil, newCodegenInternal(
			target.Loc(),
			fmt.Sprintf("named direct-choice target %q has no simple-type facts", declaredType),
			related,
			errCodegenDirectChoiceTarget,
		)
	}
	kind, err := codegenNamedScalarKind(target)
	if err != nil {
		return nil, decorateCodegenDirectChoiceError(err, element.Loc(), related)
	}
	return codegenDirectChoiceNamedTarget{
		declaredType: declaredType,
		id:           typeID,
		kind:         kind,
	}, nil
}

func decorateCodegenDirectChoiceError(err error, loc Loc, related []Loc) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return newCodegenInternal(loc, "classify direct-choice scalar target failure", related, err)
	}
	decorated := diagnostic
	decorated.loc = loc
	decorated.related = mergeCodegenRelated(related, diagnostic.related)
	return decorated
}

type codegenDirectChoiceReference uint8

const (
	codegenDirectChoiceParticlesReference codegenDirectChoiceReference = iota
	codegenDirectChoiceElementChoiceReference
	codegenDirectChoiceParticleDetailsReference
)

func codegenDirectChoiceSpecReference(version XSDVersion, reference codegenDirectChoiceReference) string {
	if version == XSDVersion10 {
		switch reference {
		case codegenDirectChoiceParticlesReference:
			return codegenDirectChoiceXSD10ParticlesSpecRef
		case codegenDirectChoiceElementChoiceReference:
			return codegenDirectChoiceXSD10ElementChoiceSpecRef
		case codegenDirectChoiceParticleDetailsReference:
			return codegenDirectChoiceXSD10ParticleDetailsSpecRef
		default:
			return codegenDirectChoiceXSD10ParticlesSpecRef
		}
	}
	switch reference {
	case codegenDirectChoiceParticlesReference:
		return codegenDirectChoiceXSD11ParticlesSpecRef
	case codegenDirectChoiceElementChoiceReference:
		return codegenDirectChoiceXSD11ElementChoiceSpecRef
	case codegenDirectChoiceParticleDetailsReference:
		return codegenDirectChoiceXSD11ParticleDetailsSpecRef
	default:
		return codegenDirectChoiceXSD11ParticlesSpecRef
	}
}

//nolint:unparam // The private helper keeps related locations aligned with the shared diagnostic boundary.
func newCodegenDirectChoiceUnsupported(
	loc Loc,
	message string,
	related []Loc,
	cause error,
	version XSDVersion,
	reference codegenDirectChoiceReference,
) error {
	return newCodegenUnsupportedForReference(
		loc,
		message,
		related,
		cause,
		version,
		codegenDirectChoiceSpecReference(version, reference),
	)
}

func newCodegenSchemaInvalid(loc Loc, message string, cause error) Diagnostic {
	return newDiagnostic(FailureInvalid, diagnosticCodegenSchemaInvalid, loc, message, cause)
}

func newCodegenDirectChoiceResolution(loc Loc, message string, related []Loc, cause error) error {
	diagnostic := newDiagnostic(FailureResolution, diagnosticCodegenSchemaInvalid, loc, message, cause)
	diagnostic.related = append([]Loc(nil), related...)
	return diagnostic
}

func codegenDirectChoiceNamingInput(
	packageName string,
	schema Schema,
	owners []codegenDirectChoiceCollectedOwner,
) codegenNamingInput {
	fieldRequests := make([]codegenLocalParticleRequest, 0)
	variantRequests := make([]codegenVariantRequest, 0)
	for _, owner := range owners {
		for _, alternative := range owner.alternatives {
			fieldRequests = append(fieldRequests, codegenLocalParticleRequest{
				owner: owner.id,
				path:  cloneCodegenPath(alternative.path),
				name:  alternative.name,
				loc:   alternative.loc,
			})
			variantRequests = append(variantRequests, codegenVariantRequest{
				owner: owner.id,
				path:  cloneCodegenPath(alternative.path),
				name:  alternative.name,
				loc:   alternative.loc,
			})
		}
	}
	return codegenNamingInput{
		packageName:    packageName,
		schema:         schema,
		localParticles: fieldRequests,
		variants:       variantRequests,
		importAliases: []codegenImportAliasRequest{{
			identity: codegenRuntimeImportPath,
			alias:    "runtime",
		}},
	}
}

//nolint:gocognit // Replay every ordered naming scope against its source input.
func validateCodegenDirectChoiceNaming(
	components []Component,
	owners []codegenDirectChoiceCollectedOwner,
	names codegenNaming,
) error {
	if err := validateCodegenNaming(components, names); err != nil {
		return err
	}
	expected := 0
	for _, owner := range owners {
		expected += len(owner.alternatives)
	}
	if len(names.fields) != expected || len(names.fieldByKey) != expected {
		return newCodegenNamingInvariant(
			Loc{},
			"direct-choice naming table field records do not match ordered input",
			errCodegenDirectChoiceNaming,
		)
	}
	if len(names.variants) != expected || len(names.variantByKey) != expected {
		return newCodegenNamingInvariant(
			Loc{},
			"direct-choice naming table variant records do not match ordered input",
			errCodegenDirectChoiceNaming,
		)
	}
	fieldIndex := 0
	variantIndex := 0
	for _, owner := range owners {
		ownerIdentifier, ok := names.componentName(owner.id)
		if !ok {
			return newCodegenNamingInvariant(owner.loc, "direct-choice owner has no generated component identifier", errCodegenDirectChoiceNaming)
		}
		if ownerIdentifier == "" {
			return newCodegenNamingInvariant(owner.loc, "direct-choice owner has an empty generated component identifier", errCodegenDirectChoiceNaming)
		}
		for _, alternative := range owner.alternatives {
			field := names.fields[fieldIndex]
			if field.owner != owner.id || !equalCodegenPath(field.path, alternative.path) || field.name != alternative.name {
				return newCodegenNamingInvariant(alternative.loc, "direct-choice field name record does not match ordered input", errCodegenDirectChoiceNaming)
			}
			fieldIdentifier, fieldOK := names.fieldName(owner.id, alternative.path)
			if !fieldOK || fieldIdentifier != field.identifier {
				return newCodegenNamingInvariant(alternative.loc, "direct-choice field lookup is stale", errCodegenDirectChoiceNaming)
			}
			variant := names.variants[variantIndex]
			if variant.owner != owner.id || !equalCodegenPath(variant.path, alternative.path) || variant.name != alternative.name {
				return newCodegenNamingInvariant(alternative.loc, "direct-choice variant name record does not match ordered input", errCodegenDirectChoiceNaming)
			}
			variantIdentifier, variantOK := names.variantName(owner.id, alternative.path)
			if !variantOK || variantIdentifier != variant.identifier {
				return newCodegenNamingInvariant(alternative.loc, "direct-choice variant lookup is stale", errCodegenDirectChoiceNaming)
			}
			fieldIndex++
			variantIndex++
		}
	}
	if fieldIndex != len(names.fields) || variantIndex != len(names.variants) {
		return newCodegenNamingInvariant(Loc{}, "direct-choice naming table contains unrequested scoped names", errCodegenDirectChoiceNaming)
	}
	if len(names.imports) != 1 {
		return newCodegenNamingInvariant(Loc{}, "direct-choice naming table runtime import records do not match ordered input", errCodegenDirectChoiceNaming)
	}
	imported := names.imports[0]
	if imported.identity != codegenRuntimeImportPath || imported.alias != "runtime" {
		return newCodegenNamingInvariant(Loc{}, "direct-choice runtime import record does not match ordered input", errCodegenDirectChoiceNaming)
	}
	if identifier, ok := names.importAlias(codegenRuntimeImportPath); !ok || identifier != imported.identifier {
		return newCodegenNamingInvariant(Loc{}, "direct-choice runtime import lookup is stale", errCodegenDirectChoiceNaming)
	}
	return nil
}

//nolint:gocognit // Materialize all ordered records only after naming validation.
func materializeCodegenDirectChoicePlan(
	packageName string,
	owners []codegenDirectChoiceCollectedOwner,
	names codegenNaming,
) (codegenDirectChoicePlan, error) {
	runtimeAlias, ok := names.importAlias(codegenRuntimeImportPath)
	if !ok || runtimeAlias == "" {
		return codegenDirectChoicePlan{}, newCodegenNamingInvariant(
			Loc{},
			"direct-choice naming table has no generated runtime import alias",
			errCodegenDirectChoiceNaming,
		)
	}
	plan := codegenDirectChoicePlan{
		packageName:  packageName,
		runtimeAlias: runtimeAlias,
		names:        names.clone(),
		owners:       make([]codegenDirectChoiceOwner, 0, len(owners)),
	}
	for _, owner := range owners {
		identifier, ok := names.componentName(owner.id)
		if !ok {
			return codegenDirectChoicePlan{}, newCodegenNamingInvariant(owner.loc, "direct-choice owner has no generated component identifier", errCodegenDirectChoiceNaming)
		}
		materializedOwner := codegenDirectChoiceOwner{
			id:           owner.id,
			name:         owner.name,
			loc:          owner.loc,
			identifier:   identifier,
			marker:       codegenDirectChoiceMarkerIdentifier(identifier),
			choiceLoc:    owner.choiceLoc,
			alternatives: make([]codegenDirectChoiceAlternative, 0, len(owner.alternatives)),
		}
		for _, alternative := range owner.alternatives {
			fieldIdentifier, fieldOK := names.fieldName(owner.id, alternative.path)
			if !fieldOK {
				return codegenDirectChoicePlan{}, newCodegenNamingInvariant(alternative.loc, "direct-choice alternative has no generated field identifier", errCodegenDirectChoiceNaming)
			}
			variantIdentifier, variantOK := names.variantName(owner.id, alternative.path)
			if !variantOK {
				return codegenDirectChoicePlan{}, newCodegenNamingInvariant(alternative.loc, "direct-choice alternative has no generated variant identifier", errCodegenDirectChoiceNaming)
			}
			target, targetErr := materializeCodegenDirectChoiceTarget(alternative.target, alternative.loc, names)
			if targetErr != nil {
				return codegenDirectChoicePlan{}, targetErr
			}
			materializedOwner.alternatives = append(materializedOwner.alternatives, codegenDirectChoiceAlternative{
				path:              cloneCodegenPath(alternative.path),
				loc:               alternative.loc,
				name:              alternative.name,
				fieldIdentifier:   fieldIdentifier,
				variantIdentifier: variantIdentifier,
				target:            target,
			})
		}
		plan.owners = append(plan.owners, materializedOwner)
	}
	return plan, nil
}

func materializeCodegenDirectChoiceTarget(
	target codegenDirectChoiceTarget,
	loc Loc,
	names codegenNaming,
) (codegenDirectChoiceTarget, error) {
	switch concrete := target.(type) {
	case codegenDirectChoiceBuiltinTarget:
		return concrete, nil
	case codegenDirectChoiceNamedTarget:
		identifier, ok := names.componentName(concrete.id)
		if !ok {
			return nil, newCodegenNamingInvariant(
				loc,
				fmt.Sprintf("named direct-choice target %s has no generated component identifier", concrete.id.Source()),
				errCodegenDirectChoiceNaming,
			)
		}
		concrete.componentIdentifier = identifier
		return concrete, nil
	default:
		return nil, newCodegenInternal(
			loc,
			"direct-choice target has an unknown concrete representation",
			nil,
			errCodegenDirectChoiceTarget,
		)
	}
}

//nolint:gocognit,funlen // Validate the complete immutable plan at its phase boundary.
func validateCodegenDirectChoicePlan(schema Schema, plan codegenDirectChoicePlan) error {
	if err := validateCodegenPackageName(plan.packageName); err != nil {
		return newCodegenNamingInvariant(Loc{}, "direct-choice plan has an invalid package name", err)
	}
	if plan.names.packageIdentifier() != plan.packageName {
		return newCodegenInternal(
			Loc{},
			"direct-choice plan naming package does not match its package name",
			nil,
			errCodegenDirectChoiceNaming,
		)
	}
	components := schema.Components()
	if err := validateCodegenNaming(components, plan.names); err != nil {
		return newCodegenInternal(
			Loc{},
			"direct-choice plan naming state is invalid",
			nil,
			err,
		)
	}
	if len(plan.names.imports) != 1 || plan.names.imports[0].identity != codegenRuntimeImportPath {
		return newCodegenInternal(
			Loc{},
			"direct-choice plan naming state has unexpected imports",
			nil,
			errCodegenDirectChoiceNaming,
		)
	}
	if runtimeAlias, ok := plan.names.importAlias(codegenRuntimeImportPath); !ok || runtimeAlias != plan.runtimeAlias {
		return newCodegenInternal(
			Loc{},
			"direct-choice plan runtime alias does not match its naming state",
			nil,
			errCodegenDirectChoiceNaming,
		)
	}
	if plan.runtimeAlias == "" {
		return newCodegenNamingInvariant(Loc{}, "direct-choice plan has an empty runtime import alias", errCodegenDirectChoiceNaming)
	}
	if _, err := codegenIdentifier(plan.runtimeAlias, codegenNameKindImport, false, nil, Loc{}); err != nil {
		return newCodegenNamingInvariant(Loc{}, "direct-choice plan has an invalid runtime import alias", err)
	}
	seenOwners := make(map[ComponentID]struct{}, len(plan.owners))
	lastOwnerIndex := -1
	for _, owner := range plan.owners {
		if owner.id.IsZero() {
			return newCodegenInternal(owner.loc, "direct-choice plan owner has an empty identity", nil, errCodegenDirectChoicePlan)
		}
		if _, exists := seenOwners[owner.id]; exists {
			return newCodegenInternal(owner.loc, "direct-choice plan owner identity is repeated", nil, errCodegenDirectChoicePlan)
		}
		seenOwners[owner.id] = struct{}{}
		component, ok := schema.Lookup(owner.id)
		if !ok || component.ID() != owner.id || component.Kind() != ComponentKindComplexTypeDefinition || component.Name() != owner.name || component.Loc() != owner.loc {
			return newCodegenNamingInvariant(owner.loc, "direct-choice plan owner does not match schema identity", errCodegenDirectChoiceNaming)
		}
		if identifier, err := codegenQNameIdentifier(owner.name, codegenNameKindComponent, false, nil, owner.loc); err != nil || identifier == "" {
			if err != nil {
				return newCodegenNamingInvariant(owner.loc, "direct-choice plan owner name cannot be validated", err)
			}
			return newCodegenNamingInvariant(owner.loc, "direct-choice plan owner has an empty generated name", errCodegenDirectChoiceNaming)
		}
		if _, ok := codegenComponentAt(components, owner.id); !ok {
			return newCodegenNamingInvariant(owner.loc, "direct-choice plan owner is not in ordered schema components", errCodegenDirectChoiceNaming)
		}
		ownerIndex := codegenComponentIndex(components, owner.id)
		if ownerIndex <= lastOwnerIndex {
			return newCodegenInternal(owner.loc, "direct-choice plan owners are not in schema component order", nil, errCodegenDirectChoicePlan)
		}
		lastOwnerIndex = ownerIndex
		if owner.marker != codegenDirectChoiceMarkerIdentifier(owner.identifier) || !validCodegenDirectChoiceMarker(owner.marker) {
			return newCodegenInternal(owner.loc, "direct-choice plan owner marker is malformed", nil, errCodegenDirectChoicePlan)
		}
		if generated, ok := plan.names.componentName(owner.id); !ok || generated != owner.identifier {
			return newCodegenInternal(owner.loc, "direct-choice plan owner name does not match its naming state", nil, errCodegenDirectChoicePlan)
		}
		for index, alternative := range owner.alternatives {
			wantPath, pathErr := codegenDirectChoicePath(index)
			if pathErr != nil {
				return newCodegenInternal(alternative.loc, "validate direct-choice plan alternative path", nil, pathErr)
			}
			if !equalCodegenPath(alternative.path, wantPath) {
				return newCodegenInternal(alternative.loc, "direct-choice plan alternative path is not the ordered full particle path", nil, errCodegenDirectChoicePlan)
			}
			if err := validateCodegenDirectChoiceQName(alternative.name, alternative.loc, "direct-choice local element"); err != nil {
				return err
			}
			field, err := codegenIdentifier(alternative.name.Local(), codegenNameKindField, false, nil, alternative.loc)
			if err != nil || field == "" {
				if err != nil {
					return newCodegenNamingInvariant(alternative.loc, "direct-choice plan field name cannot be validated", err)
				}
				return newCodegenNamingInvariant(alternative.loc, "direct-choice plan field name cannot be validated", errCodegenDirectChoiceNaming)
			}
			if alternative.fieldIdentifier == "" || alternative.variantIdentifier == "" {
				return newCodegenInternal(
					alternative.loc,
					"direct-choice plan has an empty field or variant name",
					nil,
					errCodegenDirectChoicePlan,
				)
			}
			if _, err := codegenIdentifier(alternative.fieldIdentifier, codegenNameKindField, false, nil, alternative.loc); err != nil {
				return newCodegenNamingInvariant(alternative.loc, "direct-choice plan field name is not allocated", err)
			}
			if _, err := codegenIdentifier(alternative.variantIdentifier, codegenNameKindVariant, false, nil, alternative.loc); err != nil {
				return newCodegenNamingInvariant(alternative.loc, "direct-choice plan variant name is not allocated", err)
			}
			if generated, ok := plan.names.fieldName(owner.id, alternative.path); !ok || generated != alternative.fieldIdentifier {
				return newCodegenInternal(alternative.loc, "direct-choice plan field name does not match its naming state", nil, errCodegenDirectChoicePlan)
			}
			if generated, ok := plan.names.variantName(owner.id, alternative.path); !ok || generated != alternative.variantIdentifier {
				return newCodegenInternal(alternative.loc, "direct-choice plan variant name does not match its naming state", nil, errCodegenDirectChoicePlan)
			}
			if err := validateCodegenDirectChoicePlanTarget(schema, plan.names, alternative.target, alternative.loc); err != nil {
				return err
			}
		}
	}
	for _, component := range components {
		if component.Kind() != ComponentKindComplexTypeDefinition {
			continue
		}
		if _, ok := codegenDirectChoiceOwnerAt(plan.owners, component.ID()); !ok {
			return newCodegenInternal(
				component.Loc(),
				"direct-choice plan has no owner for a schema complex type",
				nil,
				errCodegenDirectChoicePlan,
			)
		}
	}
	if err := validateCodegenDirectChoicePlanScopedNames(plan); err != nil {
		return err
	}
	return nil
}

//nolint:gocognit // Replay ordered field and variant records at the plan boundary.
func validateCodegenDirectChoicePlanScopedNames(plan codegenDirectChoicePlan) error {
	want := 0
	for _, owner := range plan.owners {
		want += len(owner.alternatives)
	}
	if len(plan.names.fields) != want || len(plan.names.variants) != want {
		return newCodegenInternal(
			Loc{},
			"direct-choice plan scoped naming record count is inconsistent",
			nil,
			errCodegenDirectChoiceNaming,
		)
	}
	fieldIndex := 0
	variantIndex := 0
	for _, owner := range plan.owners {
		for _, alternative := range owner.alternatives {
			field := plan.names.fields[fieldIndex]
			if field.owner != owner.id || !equalCodegenPath(field.path, alternative.path) || field.name != alternative.name || field.identifier != alternative.fieldIdentifier {
				return newCodegenInternal(
					alternative.loc,
					"direct-choice plan field naming record does not match its alternative",
					nil,
					errCodegenDirectChoiceNaming,
				)
			}
			if identifier, ok := plan.names.fieldName(owner.id, alternative.path); !ok || identifier != field.identifier {
				return newCodegenInternal(
					alternative.loc,
					"direct-choice plan field naming lookup is stale",
					nil,
					errCodegenDirectChoiceNaming,
				)
			}
			variant := plan.names.variants[variantIndex]
			if variant.owner != owner.id || !equalCodegenPath(variant.path, alternative.path) || variant.name != alternative.name || variant.identifier != alternative.variantIdentifier {
				return newCodegenInternal(
					alternative.loc,
					"direct-choice plan variant naming record does not match its alternative",
					nil,
					errCodegenDirectChoiceNaming,
				)
			}
			if identifier, ok := plan.names.variantName(owner.id, alternative.path); !ok || identifier != variant.identifier {
				return newCodegenInternal(
					alternative.loc,
					"direct-choice plan variant naming lookup is stale",
					nil,
					errCodegenDirectChoiceNaming,
				)
			}
			fieldIndex++
			variantIndex++
		}
	}
	return nil
}

func codegenComponentAt(components []Component, id ComponentID) (Component, bool) {
	for _, component := range components {
		if component.ID() == id {
			return component, true
		}
	}
	return Component{}, false
}

func codegenComponentIndex(components []Component, id ComponentID) int {
	for index, component := range components {
		if component.ID() == id {
			return index
		}
	}
	return -1
}

func codegenDirectChoiceOwnerAt(owners []codegenDirectChoiceOwner, id ComponentID) (codegenDirectChoiceOwner, bool) {
	for _, owner := range owners {
		if owner.id == id {
			return owner, true
		}
	}
	return codegenDirectChoiceOwner{}, false
}

//nolint:gocognit // Keep concrete target invariant checks together.
func validateCodegenDirectChoicePlanTarget(schema Schema, names codegenNaming, target codegenDirectChoiceTarget, loc Loc) error {
	switch concrete := target.(type) {
	case codegenDirectChoiceBuiltinTarget:
		if concrete.declaredType.Namespace() != xsdNamespaceURI || concrete.kind != DigitDatatypeInteger && concrete.kind != DigitDatatypeDecimal {
			return newCodegenInternal(loc, "direct-choice plan built-in target facts are inconsistent", nil, errCodegenDirectChoicePlan)
		}
		switch concrete.declaredType.Local() {
		case "integer":
			if concrete.kind != DigitDatatypeInteger {
				return newCodegenInternal(loc, "direct-choice plan built-in integer target kind is inconsistent", nil, errCodegenDirectChoicePlan)
			}
		case "decimal":
			if concrete.kind != DigitDatatypeDecimal {
				return newCodegenInternal(loc, "direct-choice plan built-in decimal target kind is inconsistent", nil, errCodegenDirectChoicePlan)
			}
		default:
			return newCodegenInternal(loc, "direct-choice plan built-in target kind is inconsistent", nil, errCodegenDirectChoicePlan)
		}
		return nil
	case codegenDirectChoiceNamedTarget:
		if concrete.id.IsZero() || concrete.componentIdentifier == "" || concrete.declaredType.Namespace() == xsdNamespaceURI || concrete.kind != DigitDatatypeInteger && concrete.kind != DigitDatatypeDecimal {
			return newCodegenInternal(loc, "direct-choice plan named target facts are inconsistent", nil, errCodegenDirectChoicePlan)
		}
		component, ok := schema.Lookup(concrete.id)
		if !ok || component.ID() != concrete.id || component.Name() != concrete.declaredType || component.Kind() != ComponentKindSimpleTypeDefinition {
			return newCodegenInternal(loc, "direct-choice plan named target identity is inconsistent", appendCodegenRelated(nil, component.Loc()), errCodegenDirectChoicePlan)
		}
		identifier, ok := names.componentName(concrete.id)
		if !ok || identifier != concrete.componentIdentifier {
			return newCodegenInternal(loc, "direct-choice plan named target identifier is inconsistent", appendCodegenRelated(nil, component.Loc()), errCodegenDirectChoicePlan)
		}
		definition, ok := component.SimpleTypeDefinition()
		if !ok {
			return newCodegenInternal(loc, "direct-choice plan named target has no simple-type facts", appendCodegenRelated(nil, component.Loc()), errCodegenDirectChoicePlan)
		}
		if concrete.kind != definition.DigitFacets().Kind() {
			return newCodegenInternal(loc, "direct-choice plan named target kind is inconsistent", appendCodegenRelated(nil, component.Loc()), errCodegenDirectChoicePlan)
		}
		return nil
	default:
		return newCodegenInternal(loc, "direct-choice plan target has an unknown concrete representation", nil, errCodegenDirectChoicePlan)
	}
}

func (plan codegenDirectChoicePlan) ownerRecords() []codegenDirectChoiceOwner {
	owners := make([]codegenDirectChoiceOwner, 0, len(plan.owners))
	for _, owner := range plan.owners {
		owner.alternatives = cloneCodegenDirectChoiceAlternatives(owner.alternatives)
		owners = append(owners, owner)
	}
	return owners
}

func (plan codegenDirectChoicePlan) clone() codegenDirectChoicePlan {
	return codegenDirectChoicePlan{
		packageName:  plan.packageName,
		runtimeAlias: plan.runtimeAlias,
		names:        plan.names.clone(),
		owners:       plan.ownerRecords(),
	}
}

func codegenDirectChoiceMarkerIdentifier(ownerIdentifier string) string {
	return "is" + ownerIdentifier
}

func cloneCodegenDirectChoiceAlternatives(alternatives []codegenDirectChoiceAlternative) []codegenDirectChoiceAlternative {
	result := make([]codegenDirectChoiceAlternative, 0, len(alternatives))
	for _, alternative := range alternatives {
		alternative.path = cloneCodegenPath(alternative.path)
		result = append(result, alternative)
	}
	return result
}

func equalCodegenPath(left, right []uint32) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
