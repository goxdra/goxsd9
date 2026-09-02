package goxsd9

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	codegenDirectSequenceXSD10ParticlesSpecRef       = "xsd10-structures#cParticles"
	codegenDirectSequenceXSD10ElementSequenceSpecRef = "xsd10-structures#element-sequence"
	codegenDirectSequenceXSD10ParticleDetailsSpecRef = "xsd10-structures#Particle_details"
	codegenDirectSequenceXSD11ParticlesSpecRef       = "xsd11-structures#cParticles"
	codegenDirectSequenceXSD11ElementSequenceSpecRef = "xsd11-structures#element-sequence"
	codegenDirectSequenceXSD11ParticleDetailsSpecRef = "xsd11-structures#Particle_details"
)

var (
	errCodegenDirectSequenceQName    = errors.New("direct sequence QName is malformed")
	errCodegenDirectSequenceParticle = errors.New("direct sequence particle fact is incomplete")
	errCodegenDirectSequenceTarget   = errors.New("direct sequence scalar target fact is incomplete")
	errCodegenDirectSequenceResolve  = errors.New("direct sequence scalar target could not be resolved")
	errCodegenDirectSequenceNaming   = errors.New("direct sequence naming table is misaligned")
	errCodegenDirectSequencePlan     = errors.New("direct sequence plan invariant is broken")
	errCodegenDirectParticlePlan     = errors.New("direct particle plan invariant is broken")
)

type codegenDirectParticleKind uint8

const (
	codegenDirectParticleInvalid codegenDirectParticleKind = iota
	codegenDirectParticleChoice
	codegenDirectParticleSequence
)

// codegenDirectParticlePlan is the private, source-free plan for the direct
// choice and sequence shapes that the Go source planner can render. The
// explicit kind keeps the two declaration shapes distinct.
type codegenDirectParticlePlan struct {
	packageName  string
	runtimeAlias string
	names        codegenNaming
	owners       []codegenDirectParticleOwner
}

type codegenDirectParticleOwner struct {
	kind       codegenDirectParticleKind
	id         ComponentID
	name       QName
	loc        Loc
	identifier string
	choice     *codegenDirectChoiceOwner
	sequence   *codegenDirectSequenceOwner
}

type codegenDirectSequenceOwner struct {
	id          ComponentID
	name        QName
	loc         Loc
	identifier  string
	sequenceLoc Loc
	fields      []codegenDirectSequenceField
}

type codegenDirectSequenceField struct {
	path            []uint32
	loc             Loc
	name            QName
	fieldIdentifier string
	target          codegenSourceTarget
}

type codegenDirectSequenceCollectedOwner struct {
	id          ComponentID
	name        QName
	loc         Loc
	sequenceLoc Loc
	fields      []codegenDirectSequenceCollectedField
}

type codegenDirectSequenceCollectedField struct {
	path   []uint32
	loc    Loc
	name   QName
	target codegenSourceTarget
}

type codegenDirectParticleCollectedOwner struct {
	kind     codegenDirectParticleKind
	choice   *codegenDirectChoiceCollectedOwner
	sequence *codegenDirectSequenceCollectedOwner
}

type codegenDirectSequenceReference uint8

const (
	codegenDirectSequenceParticlesReference codegenDirectSequenceReference = iota
	codegenDirectSequenceElementReference
	codegenDirectSequenceParticleDetailsReference
)

// planCodegenDirectParticles collects, validates, names, and materializes
// every renderable direct choice or sequence in schema order.
func planCodegenDirectParticles(schema Schema, packageName string) (codegenDirectParticlePlan, error) {
	components, version, err := prepareCodegenPlanSchema(
		schema,
		packageName,
		"direct-particle code-generation schema is zero or incomplete",
	)
	if err != nil {
		return codegenDirectParticlePlan{}, err
	}
	collected, err := collectCodegenDirectParticles(schema, components, version)
	if err != nil {
		return codegenDirectParticlePlan{}, err
	}

	input := codegenDirectParticleNamingInput(packageName, schema, collected)
	names, err := newCodegenNaming(input)
	if err != nil {
		return codegenDirectParticlePlan{}, err
	}
	namingErr := validateCodegenDirectParticleNaming(components, input, names)
	if namingErr != nil {
		return codegenDirectParticlePlan{}, namingErr
	}
	plan, err := materializeCodegenDirectParticlePlan(packageName, collected, names)
	if err != nil {
		return codegenDirectParticlePlan{}, err
	}
	if err := validateCodegenDirectParticlePlan(schema, plan); err != nil {
		return codegenDirectParticlePlan{}, err
	}
	return plan, nil
}

// collectCodegenDirectParticles keeps complex owners in schema declaration
// order and retains each particle's lexical child order.
//
//nolint:gocognit // Keep direct particle collection and shape dispatch together.
func collectCodegenDirectParticles(
	schema Schema,
	components []Component,
	version XSDVersion,
) ([]codegenDirectParticleCollectedOwner, error) {
	owners := make([]codegenDirectParticleCollectedOwner, 0)
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
				errCodegenDirectParticlePlan,
			)
		}
		particle := definition.Particle()
		if particle == nil {
			return nil, newCodegenDirectParticleUnsupported(
				component.Loc(),
				fmt.Sprintf("complex type %q has no direct choice or sequence particle", component.Name()),
				nil,
				fmt.Errorf("%w: complex type has no modeled particle", errCodegenUnsupported),
				version,
				codegenDirectSequenceParticlesReference,
			)
		}
		if directChoiceTypedNilParticle(particle) {
			return nil, newCodegenInternal(
				component.Loc(),
				fmt.Sprintf("complex type %q has a typed-nil direct particle", component.Name()),
				nil,
				errCodegenDirectParticlePlan,
			)
		}
		if choice, choiceOK := directChoiceValue(particle); choiceOK {
			owner, ownerErr := collectCodegenDirectChoiceOwner(schema, component, choice, version)
			if ownerErr != nil {
				return nil, ownerErr
			}
			owners = append(owners, codegenDirectParticleCollectedOwner{
				kind:   codegenDirectParticleChoice,
				choice: &owner,
			})
			continue
		}
		if sequence, sequenceOK := directSequenceValue(particle); sequenceOK {
			if anyAttribute, anyAttributeOK := definition.AnyAttribute(); anyAttributeOK {
				return nil, newCodegenDirectSequenceUnsupported(
					anyAttribute.Loc(),
					fmt.Sprintf("complex type %q attribute wildcards are outside direct sequence generation", component.Name()),
					appendCodegenRelated(nil, sequence.Loc()),
					fmt.Errorf("%w: complex type attribute wildcard", errCodegenUnsupported),
					version,
					codegenDirectSequenceElementReference,
				)
			}
			owner, ownerErr := collectCodegenDirectSequenceOwner(schema, component, sequence, version)
			if ownerErr != nil {
				return nil, ownerErr
			}
			owners = append(owners, codegenDirectParticleCollectedOwner{
				kind:     codegenDirectParticleSequence,
				sequence: &owner,
			})
			continue
		}
		return nil, newCodegenDirectParticleUnsupported(
			component.Loc(),
			fmt.Sprintf("complex type %q particle is outside direct choice and sequence generation", component.Name()),
			nil,
			fmt.Errorf("%w: complex type particle is not a direct choice or sequence", errCodegenUnsupported),
			version,
			codegenDirectSequenceParticlesReference,
		)
	}
	return owners, nil
}

func directSequenceValue(particle Particle) (SequenceParticle, bool) {
	switch concrete := particle.(type) {
	case SequenceParticle:
		return concrete, true
	case *SequenceParticle:
		if concrete == nil {
			return SequenceParticle{}, false
		}
		return *concrete, true
	default:
		return SequenceParticle{}, false
	}
}

//nolint:gocognit,funlen // Keep direct sequence preflight in one ordered pass.
func collectCodegenDirectSequenceOwner(
	schema Schema,
	component Component,
	sequence SequenceParticle,
	version XSDVersion,
) (codegenDirectSequenceCollectedOwner, error) {
	if sequence.facts == nil {
		return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
			component.Loc(),
			fmt.Sprintf("complex type %q has an incomplete sequence particle", component.Name()),
			nil,
			errCodegenDirectSequenceParticle,
		)
	}
	occurrences := sequence.Occurrences()
	if err := validateCodegenDirectSequenceBounds(
		sequence.facts.occurrences,
		occurrences,
		sequence.Loc(),
		"sequence",
		version,
	); err != nil {
		return codegenDirectSequenceCollectedOwner{}, err
	}

	owner := codegenDirectSequenceCollectedOwner{
		id:          component.ID(),
		name:        component.Name(),
		loc:         component.Loc(),
		sequenceLoc: sequence.Loc(),
	}
	particles := sequence.Particles()
	owner.fields = make([]codegenDirectSequenceCollectedField, 0, len(particles))
	for index, particle := range particles {
		if particle == nil {
			return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
				sequence.Loc(),
				"direct-sequence child particle is nil",
				nil,
				errCodegenDirectSequenceParticle,
			)
		}
		if directChoiceTypedNilParticle(particle) {
			return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
				sequence.Loc(),
				"direct-sequence child particle is a typed-nil particle",
				nil,
				errCodegenDirectSequenceParticle,
			)
		}
		if reference, referenceOK := elementReferenceParticleValue(particle); referenceOK {
			if reference.facts == nil {
				return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
					sequence.Loc(),
					"direct-sequence element reference has incomplete particle facts",
					nil,
					errCodegenDirectSequenceParticle,
				)
			}
			return codegenDirectSequenceCollectedOwner{}, codegenDirectSequenceReferenceUnsupported(schema, reference, version)
		}
		path, pathErr := codegenDirectSequencePath(index)
		if pathErr != nil {
			return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
				sequence.Loc(),
				"construct direct-sequence child path",
				nil,
				pathErr,
			)
		}
		element, elementOK := directChoiceValueElement(particle)
		if !elementOK {
			if nestedChoice, nestedChoiceOK := directChoiceValue(particle); nestedChoiceOK && nestedChoice.facts == nil {
				return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
					sequence.Loc(),
					"direct-sequence child contains an incomplete nested choice",
					nil,
					errCodegenDirectSequenceParticle,
				)
			}
			if nestedSequence, nestedSequenceOK := directSequenceValue(particle); nestedSequenceOK && nestedSequence.facts == nil {
				return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
					sequence.Loc(),
					"direct-sequence child contains an incomplete nested sequence",
					nil,
					errCodegenDirectSequenceParticle,
				)
			}
			return codegenDirectSequenceCollectedOwner{}, newCodegenDirectSequenceUnsupported(
				sequence.Loc(),
				"nested or non-element sequence children are outside direct sequence generation",
				nil,
				fmt.Errorf("%w: sequence child is not a direct element", errCodegenUnsupported),
				version,
				codegenDirectSequenceElementReference,
			)
		}
		if element.facts == nil {
			return codegenDirectSequenceCollectedOwner{}, newCodegenInternal(
				sequence.Loc(),
				"direct-sequence element child has incomplete particle facts",
				nil,
				errCodegenDirectSequenceParticle,
			)
		}
		if err := validateCodegenDirectSequenceBounds(
			element.facts.occurrences,
			element.Occurrences(),
			element.Loc(),
			"sequence element",
			version,
		); err != nil {
			return codegenDirectSequenceCollectedOwner{}, err
		}
		if err := validateCodegenDirectSequenceElementName(element.Name(), element.Loc()); err != nil {
			return codegenDirectSequenceCollectedOwner{}, err
		}
		target, targetErr := validateCodegenDirectSequenceTarget(schema, element, version)
		if targetErr != nil {
			return codegenDirectSequenceCollectedOwner{}, targetErr
		}
		owner.fields = append(owner.fields, codegenDirectSequenceCollectedField{
			path:   cloneCodegenPath(path),
			loc:    element.Loc(),
			name:   element.Name(),
			target: target,
		})
	}
	return owner, nil
}

func codegenDirectSequencePath(index int) ([]uint32, error) {
	if index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return nil, fmt.Errorf("%w: child ordinal overflows uint32", errCodegenDirectSequencePlan)
	}
	return []uint32{1, uint32(index + 1)}, nil
}

func validateCodegenDirectSequenceBounds(
	rangeValue particleOccurrenceRange,
	occurrences ParticleOccurrenceRange,
	loc Loc,
	context string,
	version XSDVersion,
) error {
	if !rangeValue.mapsToParticle() || rangeValue.minimum.isUnbounded() ||
		!rangeValue.maximum.isUnbounded() && rangeValue.minimum.Compare(rangeValue.maximum) > 0 ||
		!occurrences.value.Equal(rangeValue) {
		return newCodegenInternal(
			loc,
			context+" occurrence bounds are inconsistent",
			nil,
			errCodegenDirectSequenceParticle,
		)
	}
	if occurrences.IsDefault() {
		return nil
	}
	return newCodegenDirectSequenceUnsupported(
		loc,
		fmt.Sprintf("%s occurrence bounds %s are outside direct sequence generation", context, occurrences),
		nil,
		fmt.Errorf("%w: non-default %s occurrence bounds", errCodegenUnsupported, context),
		version,
		codegenDirectSequenceParticleDetailsReference,
	)
}

func validateCodegenDirectSequenceElementName(name QName, loc Loc) error {
	if !utf8.ValidString(name.Namespace()) || !utf8.ValidString(name.Local()) || name.Local() == "" {
		return newCodegenInternal(
			loc,
			"direct-sequence local element name is incomplete or invalid",
			nil,
			errCodegenDirectSequenceQName,
		)
	}
	return nil
}

//nolint:gocognit,funlen // Keep scalar identity and named-target validation together.
func validateCodegenDirectSequenceTarget(
	schema Schema,
	element ElementParticle,
	version XSDVersion,
) (codegenSourceTarget, error) {
	declaredType := element.DeclaredType()
	typeID, hasTypeID := element.TypeID()
	if declaredType.IsZero() {
		if hasTypeID || !typeID.IsZero() {
			return codegenSourceTarget{}, newCodegenInternal(
				element.Loc(),
				"anonymous direct-sequence type has a synthetic component identity",
				nil,
				errCodegenDirectSequenceTarget,
			)
		}
		return codegenSourceTarget{}, newCodegenDirectSequenceUnsupported(
			element.Loc(),
			"anonymous or inline direct-sequence element types are outside direct sequence generation",
			nil,
			fmt.Errorf("%w: anonymous element type", errCodegenUnsupported),
			version,
			codegenDirectSequenceElementReference,
		)
	}
	if err := validateCodegenDirectSequenceQName(declaredType, element.Loc(), "declared type"); err != nil {
		return codegenSourceTarget{}, err
	}
	if declaredType.Namespace() == xsdNamespaceURI {
		if hasTypeID || !typeID.IsZero() {
			return codegenSourceTarget{}, newCodegenInternal(
				element.Loc(),
				fmt.Sprintf("built-in direct-sequence type %q has a synthetic component identity", declaredType),
				nil,
				errCodegenDirectSequenceTarget,
			)
		}
		var kind codegenSourceScalarKind
		switch declaredType.Local() {
		case "integer":
			kind = codegenSourceScalarInteger
		case "decimal":
			kind = codegenSourceScalarDecimal
		default:
			return codegenSourceTarget{}, newCodegenDirectSequenceUnsupported(
				element.Loc(),
				fmt.Sprintf("built-in direct-sequence type %q is outside scalar sequence generation", declaredType),
				nil,
				fmt.Errorf("%w: built-in scalar type %q", errCodegenUnsupported, declaredType),
				version,
				codegenDirectSequenceElementReference,
			)
		}
		return codegenSourceTarget{
			form:         codegenSourceTargetBuiltin,
			declaredType: declaredType,
			scalarKind:   kind,
		}, nil
	}
	if !hasTypeID {
		return codegenSourceTarget{}, newCodegenDirectSequenceResolution(
			element.Loc(),
			fmt.Sprintf("named direct-sequence type %q has no resolved component identity", declaredType),
			nil,
			errCodegenDirectSequenceResolve,
		)
	}
	if typeID.Source() == "" || typeID.Ordinal() == 0 {
		return codegenSourceTarget{}, newCodegenInternal(
			element.Loc(),
			fmt.Sprintf("named direct-sequence type %q has an invalid component identity", declaredType),
			nil,
			errCodegenDirectSequenceTarget,
		)
	}
	target, ok := schema.Lookup(typeID)
	if !ok {
		return codegenSourceTarget{}, newCodegenDirectSequenceResolution(
			element.Loc(),
			fmt.Sprintf("named direct-sequence type identity %s is absent from the schema", typeID.Source()),
			nil,
			errCodegenDirectSequenceResolve,
		)
	}
	related := appendCodegenRelated(nil, target.Loc())
	if target.ID() != typeID || target.Name() != declaredType {
		return codegenSourceTarget{}, newCodegenInternal(
			element.Loc(),
			fmt.Sprintf("named direct-sequence type identity does not match declared QName %q", declaredType),
			related,
			errCodegenDirectSequenceTarget,
		)
	}
	if target.Kind() != ComponentKindSimpleTypeDefinition {
		return codegenSourceTarget{}, newCodegenDirectSequenceUnsupported(
			element.Loc(),
			fmt.Sprintf("named direct-sequence target %q is not a supported simple type", declaredType),
			related,
			fmt.Errorf("%w: target component kind %q", errCodegenUnsupported, target.Kind()),
			version,
			codegenDirectSequenceElementReference,
		)
	}
	if target.simpleType == nil {
		return codegenSourceTarget{}, newCodegenInternal(
			element.Loc(),
			fmt.Sprintf("named direct-sequence target %q has no simple-type facts", declaredType),
			related,
			errCodegenDirectSequenceTarget,
		)
	}
	if err := validateCodegenDirectSequenceQName(target.Name(), target.Loc(), "named scalar target"); err != nil {
		return codegenSourceTarget{}, err
	}
	scalarTarget, err := codegenNamedScalarTarget(schema, target, version)
	if err != nil {
		return codegenSourceTarget{}, decorateCodegenElementError(err, element.Loc(), related)
	}
	if scalarTarget.scalarKind != codegenSourceScalarInteger && scalarTarget.scalarKind != codegenSourceScalarDecimal {
		return codegenSourceTarget{}, newCodegenDirectSequenceUnsupported(
			element.Loc(),
			fmt.Sprintf("named direct-sequence type %q is outside integer and decimal sequence generation", declaredType),
			related,
			fmt.Errorf("%w: named scalar kind %q", errCodegenUnsupported, scalarTarget.scalarKind),
			version,
			codegenDirectSequenceElementReference,
		)
	}
	scalarTarget.form = codegenSourceTargetNamed
	scalarTarget.declaredType = declaredType
	scalarTarget.typeID = typeID
	scalarTarget.hasTypeID = true
	return scalarTarget, nil
}

func validateCodegenDirectSequenceQName(name QName, loc Loc, context string) error {
	if !utf8.ValidString(name.Namespace()) || !utf8.ValidString(name.Local()) {
		return newCodegenInternal(loc, context+" QName is not valid UTF-8", nil, errCodegenDirectSequenceQName)
	}
	if name.Local() == "" {
		return newCodegenInternal(loc, context+" QName has an empty local name", nil, errCodegenDirectSequenceQName)
	}
	return nil
}

func codegenDirectSequenceReferenceUnsupported(schema Schema, reference ElementReferenceParticle, version XSDVersion) error {
	primary := reference.RefLoc()
	if primary.IsZero() {
		primary = reference.Loc()
	}
	related := appendCodegenRelated(nil, reference.Loc())
	if target, ok := schema.Lookup(reference.TargetID()); ok {
		related = appendCodegenRelated(related, target.Loc())
	}
	return newCodegenDirectSequenceUnsupported(
		primary,
		"direct sequence element reference particles are outside Go code generation",
		related,
		fmt.Errorf("%w: element reference particle", errCodegenUnsupported),
		version,
		codegenDirectSequenceElementReference,
	)
}

func codegenDirectSequenceSpecReference(version XSDVersion, reference codegenDirectSequenceReference) string {
	if version == XSDVersion10 {
		switch reference {
		case codegenDirectSequenceParticlesReference:
			return codegenDirectSequenceXSD10ParticlesSpecRef
		case codegenDirectSequenceElementReference:
			return codegenDirectSequenceXSD10ElementSequenceSpecRef
		case codegenDirectSequenceParticleDetailsReference:
			return codegenDirectSequenceXSD10ParticleDetailsSpecRef
		default:
			return codegenDirectSequenceXSD10ParticlesSpecRef
		}
	}
	switch reference {
	case codegenDirectSequenceParticlesReference:
		return codegenDirectSequenceXSD11ParticlesSpecRef
	case codegenDirectSequenceElementReference:
		return codegenDirectSequenceXSD11ElementSequenceSpecRef
	case codegenDirectSequenceParticleDetailsReference:
		return codegenDirectSequenceXSD11ParticleDetailsSpecRef
	default:
		return codegenDirectSequenceXSD11ParticlesSpecRef
	}
}

func newCodegenDirectSequenceUnsupported(
	loc Loc,
	message string,
	related []Loc,
	cause error,
	version XSDVersion,
	reference codegenDirectSequenceReference,
) error {
	return newCodegenUnsupportedForReference(
		loc,
		message,
		related,
		cause,
		version,
		codegenDirectSequenceSpecReference(version, reference),
	)
}

func newCodegenDirectParticleUnsupported(
	loc Loc,
	message string,
	related []Loc,
	cause error,
	version XSDVersion,
	reference codegenDirectSequenceReference,
) error {
	return newCodegenDirectSequenceUnsupported(loc, message, related, cause, version, reference)
}

func newCodegenDirectSequenceResolution(loc Loc, message string, related []Loc, cause error) error {
	diagnostic := newDiagnostic(FailureResolution, diagnosticCodegenSchemaInvalid, loc, message, cause)
	diagnostic.related = append([]Loc(nil), related...)
	return diagnostic
}

func codegenDirectParticleNamingInput(
	packageName string,
	schema Schema,
	owners []codegenDirectParticleCollectedOwner,
) codegenNamingInput {
	fieldRequests := make([]codegenLocalParticleRequest, 0)
	variantRequests := make([]codegenVariantRequest, 0)
	for _, owner := range owners {
		if owner.kind == codegenDirectParticleChoice {
			for _, alternative := range owner.choice.alternatives {
				fieldRequests = append(fieldRequests, codegenLocalParticleRequest{
					owner: owner.choice.id,
					path:  cloneCodegenPath(alternative.path),
					name:  alternative.name,
					loc:   alternative.loc,
				})
				variantRequests = append(variantRequests, codegenVariantRequest{
					owner: owner.choice.id,
					path:  cloneCodegenPath(alternative.path),
					name:  alternative.name,
					loc:   alternative.loc,
				})
			}
			continue
		}
		for _, field := range owner.sequence.fields {
			fieldRequests = append(fieldRequests, codegenLocalParticleRequest{
				owner: owner.sequence.id,
				path:  cloneCodegenPath(field.path),
				name:  field.name,
				loc:   field.loc,
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

func validateCodegenDirectParticleNaming(
	components []Component,
	input codegenNamingInput,
	names codegenNaming,
) error {
	if err := validateCodegenNaming(components, names); err != nil {
		return err
	}
	expected, err := newCodegenNaming(input)
	if err != nil {
		return err
	}
	if err := compareCodegenNaming(names, expected, Loc{}); err != nil {
		return newCodegenInternal(Loc{}, "direct-particle naming records do not match ordered input", nil, errCodegenDirectParticlePlan)
	}
	if len(names.imports) != 1 || names.imports[0].identity != codegenRuntimeImportPath {
		return newCodegenNamingInvariant(Loc{}, "direct-particle naming table runtime import records do not match ordered input", errCodegenDirectSequenceNaming)
	}
	return nil
}

//nolint:gocognit // Keep ordered choice and sequence materialization together.
func materializeCodegenDirectParticlePlan(
	packageName string,
	collected []codegenDirectParticleCollectedOwner,
	names codegenNaming,
) (codegenDirectParticlePlan, error) {
	runtimeAlias, ok := names.importAlias(codegenRuntimeImportPath)
	if !ok || runtimeAlias == "" {
		return codegenDirectParticlePlan{}, newCodegenNamingInvariant(
			Loc{},
			"direct-particle naming table has no generated runtime import alias",
			errCodegenDirectSequenceNaming,
		)
	}
	choiceCollected := make([]codegenDirectChoiceCollectedOwner, 0)
	for _, owner := range collected {
		if owner.kind != codegenDirectParticleChoice {
			continue
		}
		choiceCollected = append(choiceCollected, *owner.choice)
	}
	choicePlan, err := materializeCodegenDirectChoicePlan(packageName, choiceCollected, names)
	if err != nil {
		return codegenDirectParticlePlan{}, err
	}
	choiceIndex := 0
	plan := codegenDirectParticlePlan{
		packageName:  packageName,
		runtimeAlias: runtimeAlias,
		names:        names.clone(),
		owners:       make([]codegenDirectParticleOwner, 0, len(collected)),
	}
	for _, collectedOwner := range collected {
		var id ComponentID
		var name QName
		var loc Loc
		if collectedOwner.kind == codegenDirectParticleChoice {
			id = collectedOwner.choice.id
			name = collectedOwner.choice.name
			loc = collectedOwner.choice.loc
		}
		if collectedOwner.kind == codegenDirectParticleSequence {
			id = collectedOwner.sequence.id
			name = collectedOwner.sequence.name
			loc = collectedOwner.sequence.loc
		}
		identifier, ok := names.componentName(id)
		if !ok {
			return codegenDirectParticlePlan{}, newCodegenNamingInvariant(loc, "direct-particle owner has no generated component identifier", errCodegenDirectSequenceNaming)
		}
		owner := codegenDirectParticleOwner{
			kind:       collectedOwner.kind,
			id:         id,
			name:       name,
			loc:        loc,
			identifier: identifier,
		}
		if collectedOwner.kind == codegenDirectParticleChoice {
			if choiceIndex >= len(choicePlan.owners) {
				return codegenDirectParticlePlan{}, newCodegenInternal(loc, "direct-particle choice materialization is incomplete", nil, errCodegenDirectParticlePlan)
			}
			choiceOwner := choicePlan.owners[choiceIndex]
			owner.choice = &choiceOwner
			choiceIndex++
		}
		if collectedOwner.kind == codegenDirectParticleSequence {
			sequenceOwner, sequenceErr := materializeCodegenDirectSequenceOwner(*collectedOwner.sequence, names)
			if sequenceErr != nil {
				return codegenDirectParticlePlan{}, sequenceErr
			}
			owner.sequence = &sequenceOwner
		}
		plan.owners = append(plan.owners, owner)
	}
	if choiceIndex != len(choicePlan.owners) {
		return codegenDirectParticlePlan{}, newCodegenInternal(
			codegenDirectParticlePlanLoc(plan),
			"direct-particle choice materialization has extra owners",
			nil,
			errCodegenDirectParticlePlan,
		)
	}
	return plan, nil
}

func materializeCodegenDirectSequenceOwner(
	collected codegenDirectSequenceCollectedOwner,
	names codegenNaming,
) (codegenDirectSequenceOwner, error) {
	owner := codegenDirectSequenceOwner{
		id:          collected.id,
		name:        collected.name,
		loc:         collected.loc,
		sequenceLoc: collected.sequenceLoc,
		fields:      make([]codegenDirectSequenceField, 0, len(collected.fields)),
	}
	identifier, ok := names.componentName(collected.id)
	if !ok {
		return codegenDirectSequenceOwner{}, newCodegenNamingInvariant(collected.loc, "direct-sequence owner has no generated component identifier", errCodegenDirectSequenceNaming)
	}
	owner.identifier = identifier
	for _, collectedField := range collected.fields {
		fieldIdentifier, ok := names.fieldName(collected.id, collectedField.path)
		if !ok {
			return codegenDirectSequenceOwner{}, newCodegenNamingInvariant(collectedField.loc, "direct-sequence field has no generated identifier", errCodegenDirectSequenceNaming)
		}
		owner.fields = append(owner.fields, codegenDirectSequenceField{
			path:            cloneCodegenPath(collectedField.path),
			loc:             collectedField.loc,
			name:            collectedField.name,
			fieldIdentifier: fieldIdentifier,
			target:          collectedField.target,
		})
	}
	return owner, nil
}

func validateCodegenDirectParticlePlan(schema Schema, plan codegenDirectParticlePlan) error {
	if err := validateCodegenPackageName(plan.packageName); err != nil {
		return newCodegenInternal(codegenDirectParticlePlanLoc(plan), "direct-particle plan has an invalid package name", nil, errCodegenDirectParticlePlan)
	}
	if plan.names.packageIdentifier() != plan.packageName {
		return newCodegenInternal(codegenDirectParticlePlanLoc(plan), "direct-particle plan naming package does not match its package name", nil, errCodegenDirectParticlePlan)
	}
	if schema.storage == nil {
		return newDiagnostic(
			FailureInvalid,
			diagnosticCodegenSchemaInvalid,
			Loc{},
			"direct-particle code-generation schema is zero or incomplete",
			errCodegenSchemaEmpty,
		)
	}
	components := schema.Components()
	if err := validateCodegenSchemaStorage(schema, components); err != nil {
		return err
	}
	version, err := codegenSchemaVersion(schema)
	if err != nil {
		return err
	}
	collected, err := collectCodegenDirectParticles(schema, components, version)
	if err != nil {
		return err
	}
	input := codegenDirectParticleNamingInput(plan.packageName, schema, collected)
	expectedNames, err := newCodegenNaming(input)
	if err != nil {
		return err
	}
	expected, err := materializeCodegenDirectParticlePlan(plan.packageName, collected, expectedNames)
	if err != nil {
		return err
	}
	if err := validateCodegenNaming(components, plan.names); err != nil {
		return newCodegenInternal(codegenDirectParticlePlanLoc(plan), "direct-particle plan naming state is invalid", nil, err)
	}
	if err := validateCodegenScopedNamingIndexes(plan.names, codegenDirectParticlePlanLoc(plan)); err != nil {
		return err
	}
	if runtimeAlias, ok := plan.names.importAlias(codegenRuntimeImportPath); !ok || runtimeAlias != plan.runtimeAlias || plan.runtimeAlias == "" {
		return newCodegenInternal(codegenDirectParticlePlanLoc(plan), "direct-particle plan runtime alias does not match its naming state", nil, errCodegenDirectParticlePlan)
	}
	if _, err := codegenIdentifier(plan.runtimeAlias, codegenNameKindImport, false, nil, codegenDirectParticlePlanLoc(plan)); err != nil {
		return newCodegenInternal(codegenDirectParticlePlanLoc(plan), "direct-particle plan runtime alias is invalid", nil, errCodegenDirectParticlePlan)
	}
	if err := compareCodegenNaming(plan.names, expected.names, codegenDirectParticlePlanLoc(plan)); err != nil {
		return err
	}
	return compareCodegenDirectParticlePlans(plan, expected)
}

//nolint:gocognit // Keep complete ordered direct-particle comparison together.
func compareCodegenDirectParticlePlans(actual, expected codegenDirectParticlePlan) error {
	if actual.packageName != expected.packageName || actual.runtimeAlias != expected.runtimeAlias {
		return newCodegenInternal(codegenDirectParticlePlanLoc(expected), "direct-particle plan header does not match the schema", nil, errCodegenDirectParticlePlan)
	}
	if len(actual.owners) != len(expected.owners) {
		if len(actual.owners) < len(expected.owners) {
			return newCodegenInternal(codegenDirectParticleCollectedOwnerLoc(expected.owners, len(actual.owners)), "direct-particle plan is missing a schema owner", nil, errCodegenDirectParticlePlan)
		}
		return newCodegenInternal(codegenDirectParticleOwnerLoc(actual, len(expected.owners)), "direct-particle plan contains an extra owner not present in the schema", nil, errCodegenDirectParticlePlan)
	}
	for index, expectedOwner := range expected.owners {
		actualOwner := actual.owners[index]
		ownerLoc := expectedOwner.loc
		if ownerLoc.IsZero() {
			ownerLoc = actualOwner.loc
		}
		if actualOwner.kind != expectedOwner.kind || actualOwner.id != expectedOwner.id || actualOwner.name != expectedOwner.name || actualOwner.loc != expectedOwner.loc || actualOwner.identifier != expectedOwner.identifier {
			return newCodegenInternal(ownerLoc, "direct-particle plan owner facts do not match the schema", nil, errCodegenDirectParticlePlan)
		}
		if actualOwner.kind == codegenDirectParticleChoice {
			if actualOwner.choice == nil || actualOwner.sequence != nil || expectedOwner.choice == nil || expectedOwner.sequence != nil {
				return newCodegenInternal(ownerLoc, "direct-particle choice owner shape is inconsistent", nil, errCodegenDirectParticlePlan)
			}
			if err := compareCodegenDirectParticleChoiceOwner(actualOwner.choice, expectedOwner.choice, ownerLoc); err != nil {
				return err
			}
			continue
		}
		if actualOwner.kind == codegenDirectParticleSequence {
			if actualOwner.sequence == nil || actualOwner.choice != nil || expectedOwner.sequence == nil || expectedOwner.choice != nil {
				return newCodegenInternal(ownerLoc, "direct-particle sequence owner shape is inconsistent", nil, errCodegenDirectParticlePlan)
			}
			if err := compareCodegenDirectSequenceOwner(actualOwner.sequence, expectedOwner.sequence, ownerLoc); err != nil {
				return err
			}
			continue
		}
		return newCodegenInternal(ownerLoc, "direct-particle plan has an unknown declaration shape", nil, errCodegenDirectParticlePlan)
	}
	return nil
}

func compareCodegenDirectParticleChoiceOwner(actual, expected *codegenDirectChoiceOwner, loc Loc) error {
	if actual == nil || expected == nil {
		return newCodegenInternal(loc, "direct-particle choice owner presence does not match the schema", nil, errCodegenDirectParticlePlan)
	}
	if actual.id != expected.id || actual.name != expected.name || actual.loc != expected.loc || actual.identifier != expected.identifier || actual.marker != expected.marker || actual.choiceLoc != expected.choiceLoc || len(actual.alternatives) != len(expected.alternatives) {
		return newCodegenInternal(loc, "direct-particle choice owner facts do not match the schema", nil, errCodegenDirectParticlePlan)
	}
	for index, expectedAlternative := range expected.alternatives {
		actualAlternative := actual.alternatives[index]
		if !equalCodegenPath(actualAlternative.path, expectedAlternative.path) || actualAlternative.loc != expectedAlternative.loc || actualAlternative.name != expectedAlternative.name || actualAlternative.fieldIdentifier != expectedAlternative.fieldIdentifier || actualAlternative.variantIdentifier != expectedAlternative.variantIdentifier {
			return newCodegenInternal(expectedAlternative.loc, "direct-particle choice alternative facts do not match the schema", nil, errCodegenDirectParticlePlan)
		}
		if err := validateCodegenDirectChoicePlanTargetMatches(expectedAlternative.target, actualAlternative.target, expectedAlternative.loc); err != nil {
			return newCodegenInternal(expectedAlternative.loc, "direct-particle choice target facts do not match the schema", nil, errCodegenDirectParticlePlan)
		}
	}
	return nil
}

func compareCodegenDirectSequenceOwner(actual, expected *codegenDirectSequenceOwner, loc Loc) error {
	if actual == nil || expected == nil {
		return newCodegenInternal(loc, "direct-particle sequence owner presence does not match the schema", nil, errCodegenDirectParticlePlan)
	}
	if actual.id != expected.id || actual.name != expected.name || actual.loc != expected.loc || actual.identifier != expected.identifier || actual.sequenceLoc != expected.sequenceLoc || len(actual.fields) != len(expected.fields) {
		return newCodegenInternal(loc, "direct-particle sequence owner facts do not match the schema", nil, errCodegenDirectParticlePlan)
	}
	for index, expectedField := range expected.fields {
		actualField := actual.fields[index]
		if !equalCodegenPath(actualField.path, expectedField.path) || actualField.loc != expectedField.loc || actualField.name != expectedField.name || actualField.fieldIdentifier != expectedField.fieldIdentifier || actualField.target != expectedField.target {
			return newCodegenInternal(expectedField.loc, "direct-particle sequence field facts do not match the schema", nil, errCodegenDirectParticlePlan)
		}
	}
	return nil
}

func codegenDirectParticleCollectedOwnerLoc(owners []codegenDirectParticleOwner, index int) Loc {
	if index >= 0 && index < len(owners) {
		return owners[index].loc
	}
	return Loc{}
}

func codegenDirectParticleOwnerLoc(plan codegenDirectParticlePlan, index int) Loc {
	if index >= 0 && index < len(plan.owners) {
		return plan.owners[index].loc
	}
	return codegenDirectParticlePlanLoc(plan)
}

func codegenDirectParticlePlanLoc(plan codegenDirectParticlePlan) Loc {
	for _, owner := range plan.owners {
		if !owner.loc.IsZero() {
			return owner.loc
		}
		if owner.choice != nil && !owner.choice.choiceLoc.IsZero() {
			return owner.choice.choiceLoc
		}
		if owner.sequence != nil && !owner.sequence.sequenceLoc.IsZero() {
			return owner.sequence.sequenceLoc
		}
	}
	return Loc{}
}

func codegenDirectParticleOwnerAt(owners []codegenDirectParticleOwner, id ComponentID) (codegenDirectParticleOwner, bool) {
	for _, owner := range owners {
		if owner.id == id {
			return owner, true
		}
	}
	return codegenDirectParticleOwner{}, false
}
