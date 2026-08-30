package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/goxdra/goxsd9"
)

const (
	instancePlanErrorCode           = "catalog.instance.plan"
	instanceExecutionContextCode    = "execution.instance.context"
	instanceExecutionFilesystemCode = "execution.instance.filesystem"
	instanceExecutionResourceCode   = "execution.instance"
)

const (
	auxiliaryInstanceVersion10 = "1.0"
	auxiliaryInstanceVersion11 = "1.1"
)

// InstanceKey identifies one catalog instance case in one exact XSD edition.
// The key is independent of the instance and schema resource paths so catalog
// metadata remains the source of those paths.
type InstanceKey struct {
	SetPath   string
	GroupName string
	CaseName  string
	Version   string
}

// EffectiveExpectationOverride changes one source-catalog expectation for a
// precisely identified instance case. SourceValidity is a drift guard; it is
// not a second catalog oracle.
type EffectiveExpectationOverride struct {
	Key               InstanceKey
	SourceValidity    string
	EffectiveValidity string
}

// EffectiveExpectationPolicy is an immutable sparse set of effective
// expectation overrides. A zero policy derives every effective expectation
// from the source catalog.
type EffectiveExpectationPolicy struct {
	overrides []EffectiveExpectationOverride
}

// NewEffectiveExpectationPolicy copies overrides into an immutable policy.
// Entries are validated when an inventory creates an execution plan so a
// malformed policy can be tested without opening any execution resource.
func NewEffectiveExpectationPolicy(overrides []EffectiveExpectationOverride) EffectiveExpectationPolicy {
	return EffectiveExpectationPolicy{
		overrides: append([]EffectiveExpectationOverride(nil), overrides...),
	}
}

// Overrides returns a copy of the policy's sparse overrides in declaration
// order.
func (policy EffectiveExpectationPolicy) Overrides() []EffectiveExpectationOverride {
	return append([]EffectiveExpectationOverride(nil), policy.overrides...)
}

// EffectiveExpectation returns the exact override for key, or sourceValidity
// when the policy has no matching override. Plan construction validates keys
// and source validity before relying on this fallback.
func (policy EffectiveExpectationPolicy) EffectiveExpectation(key InstanceKey, sourceValidity string) string {
	for _, override := range policy.overrides {
		if override.Key == key {
			return override.EffectiveValidity
		}
	}
	return sourceValidity
}

// AuxiliaryInstanceCase is an immutable, ordered case in an auxiliary
// instance replay plan.
type AuxiliaryInstanceCase struct {
	key               InstanceKey
	setName           string
	instancePath      string
	pairedSchema      catalogCase
	origin            catalogOrigin
	status            catalogStatus
	version           string
	policy            goxsd9.LanguagePolicy
	sourceValidity    string
	effectiveValidity string
}

// Key returns the exact catalog identity of the instance case.
func (caseValue AuxiliaryInstanceCase) Key() InstanceKey {
	return caseValue.key
}

// SetPath returns the pinned catalog test-set path.
func (caseValue AuxiliaryInstanceCase) SetPath() string {
	return caseValue.key.SetPath
}

// SetName returns the catalog test-set identity.
func (caseValue AuxiliaryInstanceCase) SetName() string {
	return caseValue.setName
}

// GroupName returns the catalog test-group identity shared by the instance
// and its paired schema test.
func (caseValue AuxiliaryInstanceCase) GroupName() string {
	return caseValue.key.GroupName
}

// CaseName returns the catalog instance-test identity.
func (caseValue AuxiliaryInstanceCase) CaseName() string {
	return caseValue.key.CaseName
}

// SchemaName returns the paired catalog schema-test identity.
func (caseValue AuxiliaryInstanceCase) SchemaName() string {
	return caseValue.pairedSchema.name
}

// Origin returns the catalog provenance.
func (caseValue AuxiliaryInstanceCase) Origin() string {
	return string(caseValue.origin)
}

// Status returns the catalog's current status.
func (caseValue AuxiliaryInstanceCase) Status() string {
	return string(caseValue.status)
}

// Version returns the exact catalog edition used for the case.
func (caseValue AuxiliaryInstanceCase) Version() string {
	return caseValue.version
}

// Policy returns the strict parser policy selected before execution.
func (caseValue AuxiliaryInstanceCase) Policy() goxsd9.LanguagePolicy {
	return caseValue.policy
}

// SchemaPaths returns paired schema-document paths in catalog order.
func (caseValue AuxiliaryInstanceCase) SchemaPaths() []string {
	return append([]string(nil), caseValue.pairedSchema.documents...)
}

// SchemaPath returns the first paired schema-document path, or an empty string
// when the plan entry has no schema document.
func (caseValue AuxiliaryInstanceCase) SchemaPath() string {
	if len(caseValue.pairedSchema.documents) == 0 {
		return ""
	}
	return caseValue.pairedSchema.documents[0]
}

// InstancePath returns the pinned instance-document path.
func (caseValue AuxiliaryInstanceCase) InstancePath() string {
	return caseValue.instancePath
}

// SourceExpectedValidity returns the immutable source-catalog expectation.
func (caseValue AuxiliaryInstanceCase) SourceExpectedValidity() string {
	return caseValue.sourceValidity
}

// EffectiveExpectedValidity returns the expectation selected by the injected
// effective-expectation policy.
func (caseValue AuxiliaryInstanceCase) EffectiveExpectedValidity() string {
	return caseValue.effectiveValidity
}

// AuxiliaryInstancePlan is an immutable, bounded replay plan in catalog
// order. It contains only accepted, non-queried auxiliary instance cases.
type AuxiliaryInstancePlan struct {
	policy EffectiveExpectationPolicy
	cases  []AuxiliaryInstanceCase
}

// PlanAuxiliaryInstances validates and selects auxiliary instance cases in
// catalog order. It validates the complete policy and every schema pairing
// before returning any plan, and never opens a schema or instance resource.
func (inventory Inventory) PlanAuxiliaryInstances(policy EffectiveExpectationPolicy) (AuxiliaryInstancePlan, error) {
	if err := validateEffectiveExpectationPolicy(policy, inventory.cases); err != nil {
		return AuxiliaryInstancePlan{}, err
	}

	planned := make([]AuxiliaryInstanceCase, 0)
	for _, instance := range inventory.cases {
		if !isAuxiliaryInstanceCandidate(instance) {
			continue
		}
		version, err := uniqueAuxiliaryInstanceVersion(instance)
		if err != nil {
			return AuxiliaryInstancePlan{}, instancePlanError(instance.setPath, err)
		}
		languagePolicy, err := LanguagePolicyForVersions([]string{version})
		if err != nil {
			return AuxiliaryInstancePlan{}, instancePlanError(instance.setPath, err)
		}
		_, sourceValidity, usable, reasons := caseFacts(instance, version)
		if !usable {
			return AuxiliaryInstancePlan{}, instancePlanError(instance.setPath,
				fmt.Errorf("instance case %q is not usable for version %q: %s",
					instance.name, version, strings.Join(reasons, ",")))
		}
		if len(instance.documents) != 1 {
			return AuxiliaryInstancePlan{}, instancePlanError(instance.setPath,
				fmt.Errorf("instance case %q has %d instance documents, want one",
					instance.name, len(instance.documents)))
		}

		schema, err := inventory.pairedAuxiliarySchema(instance, version)
		if err != nil {
			return AuxiliaryInstancePlan{}, instancePlanError(instance.setPath, err)
		}
		key := InstanceKey{
			SetPath:   instance.setPath,
			GroupName: instance.groupName,
			CaseName:  instance.name,
			Version:   version,
		}
		effectiveValidity := policy.EffectiveExpectation(key, sourceValidity)
		planned = append(planned, AuxiliaryInstanceCase{
			key:               key,
			setName:           instance.setName,
			instancePath:      instance.documents[0],
			pairedSchema:      cloneCatalogCase(schema),
			origin:            instance.origin,
			status:            instance.status,
			version:           version,
			policy:            languagePolicy,
			sourceValidity:    sourceValidity,
			effectiveValidity: effectiveValidity,
		})
	}
	if len(planned) == 0 {
		return AuxiliaryInstancePlan{}, instancePlanError("",
			errors.New("auxiliary instance plan contains zero selected cases"))
	}
	return AuxiliaryInstancePlan{
		policy: NewEffectiveExpectationPolicy(policy.overrides),
		cases:  planned,
	}, nil
}

// Policy returns an immutable copy of the injected effective-expectation
// policy.
func (plan AuxiliaryInstancePlan) Policy() EffectiveExpectationPolicy {
	return NewEffectiveExpectationPolicy(plan.policy.overrides)
}

// Len returns the number of ordered auxiliary instance cases.
func (plan AuxiliaryInstancePlan) Len() int {
	return len(plan.cases)
}

// Cases returns the ordered plan cases as independent copies.
func (plan AuxiliaryInstancePlan) Cases() []AuxiliaryInstanceCase {
	result := make([]AuxiliaryInstanceCase, 0, len(plan.cases))
	for _, caseValue := range plan.cases {
		result = append(result, cloneAuxiliaryInstanceCase(caseValue))
	}
	return result
}

// Case returns the plan case at index and reports whether it exists.
func (plan AuxiliaryInstancePlan) Case(index int) (AuxiliaryInstanceCase, bool) {
	if index < 0 || index >= len(plan.cases) {
		return AuxiliaryInstanceCase{}, false
	}
	return cloneAuxiliaryInstanceCase(plan.cases[index]), true
}

// AuxiliaryStageResult is the immutable result of one schema or instance
// stage. It keeps parser diagnostics and their causes in processing order.
type AuxiliaryStageResult struct {
	actual          ActualResult
	actualClass     goxsd9.FailureClass
	outcome         Outcome
	diagnostics     []goxsd9.Diagnostic
	err             error
	executionReason string
}

// Executed reports whether the stage was attempted. Resource-boundary failures
// are attempted stages even when the public parser or validator was not called.
func (result AuxiliaryStageResult) Executed() bool {
	return result.actual != "" && result.actual != ActualNotExecuted
}

// Actual returns the validity established by the stage.
func (result AuxiliaryStageResult) Actual() ActualResult {
	return result.actual
}

// ActualClass returns the stage's failure class, when one exists.
func (result AuxiliaryStageResult) ActualClass() goxsd9.FailureClass {
	return result.actualClass
}

// Outcome returns the independently classified stage outcome.
func (result AuxiliaryStageResult) Outcome() Outcome {
	return result.outcome
}

// Diagnostics returns stage diagnostics in processing order.
func (result AuxiliaryStageResult) Diagnostics() []goxsd9.Diagnostic {
	return append([]goxsd9.Diagnostic(nil), result.diagnostics...)
}

// Cause returns the stage's parser or resource cause.
func (result AuxiliaryStageResult) Cause() error {
	return result.err
}

// ExecutionReason returns a stable reason when the stage did not execute.
func (result AuxiliaryStageResult) ExecutionReason() string {
	return result.executionReason
}

// AuxiliaryInstanceResult contains separate schema and instance outcomes for
// one ordered auxiliary replay case.
type AuxiliaryInstanceResult struct {
	caseValue      AuxiliaryInstanceCase
	schemaStage    AuxiliaryStageResult
	instanceStage  AuxiliaryStageResult
	sourceMatch    bool
	effectiveMatch bool
}

// Key returns the exact catalog identity of the result.
func (result AuxiliaryInstanceResult) Key() InstanceKey {
	return result.caseValue.Key()
}

// SetPath returns the pinned catalog test-set path.
func (result AuxiliaryInstanceResult) SetPath() string {
	return result.caseValue.SetPath()
}

// SetName returns the catalog test-set identity.
func (result AuxiliaryInstanceResult) SetName() string {
	return result.caseValue.SetName()
}

// GroupName returns the catalog test-group identity.
func (result AuxiliaryInstanceResult) GroupName() string {
	return result.caseValue.GroupName()
}

// CaseName returns the catalog instance-test identity.
func (result AuxiliaryInstanceResult) CaseName() string {
	return result.caseValue.CaseName()
}

// Origin returns the catalog provenance.
func (result AuxiliaryInstanceResult) Origin() string {
	return result.caseValue.Origin()
}

// Status returns the catalog's current status.
func (result AuxiliaryInstanceResult) Status() string {
	return result.caseValue.Status()
}

// Version returns the exact catalog edition used for the case.
func (result AuxiliaryInstanceResult) Version() string {
	return result.caseValue.Version()
}

// Policy returns the strict parser policy selected for the case.
func (result AuxiliaryInstanceResult) Policy() goxsd9.LanguagePolicy {
	return result.caseValue.Policy()
}

// SchemaPaths returns paired schema-document paths in catalog order.
func (result AuxiliaryInstanceResult) SchemaPaths() []string {
	return result.caseValue.SchemaPaths()
}

// SchemaPath returns the first paired schema-document path.
func (result AuxiliaryInstanceResult) SchemaPath() string {
	return result.caseValue.SchemaPath()
}

// InstancePath returns the pinned instance-document path.
func (result AuxiliaryInstanceResult) InstancePath() string {
	return result.caseValue.InstancePath()
}

// SourceExpectedValidity returns the source-catalog expectation.
func (result AuxiliaryInstanceResult) SourceExpectedValidity() string {
	return result.caseValue.SourceExpectedValidity()
}

// EffectiveExpectedValidity returns the injected effective replay expectation.
func (result AuxiliaryInstanceResult) EffectiveExpectedValidity() string {
	return result.caseValue.EffectiveExpectedValidity()
}

// SourceMatch reports whether the instance actual validity matches the source
// catalog expectation.
func (result AuxiliaryInstanceResult) SourceMatch() bool {
	return result.sourceMatch
}

// EffectiveMatch reports whether the instance actual validity matches the
// effective replay expectation.
func (result AuxiliaryInstanceResult) EffectiveMatch() bool {
	return result.effectiveMatch
}

// SchemaStage returns an immutable copy of the schema-stage result.
func (result AuxiliaryInstanceResult) SchemaStage() AuxiliaryStageResult {
	return cloneAuxiliaryStageResult(result.schemaStage)
}

// InstanceStage returns an immutable copy of the instance-stage result.
func (result AuxiliaryInstanceResult) InstanceStage() AuxiliaryStageResult {
	return cloneAuxiliaryStageResult(result.instanceStage)
}

// HeadlineEligible reports that auxiliary evidence never contributes to the
// headline conformance inventory.
func (result AuxiliaryInstanceResult) HeadlineEligible() bool {
	return false
}

// AuxiliaryInstanceReport is the deterministic result of an auxiliary
// instance replay.
type AuxiliaryInstanceReport struct {
	policy EffectiveExpectationPolicy
	cases  []AuxiliaryInstanceResult
}

// Policy returns an immutable copy of the replay policy.
func (report AuxiliaryInstanceReport) Policy() EffectiveExpectationPolicy {
	return NewEffectiveExpectationPolicy(report.policy.overrides)
}

// Len returns the number of ordered result cases.
func (report AuxiliaryInstanceReport) Len() int {
	return len(report.cases)
}

// Cases returns result cases as independent copies.
func (report AuxiliaryInstanceReport) Cases() []AuxiliaryInstanceResult {
	result := make([]AuxiliaryInstanceResult, 0, len(report.cases))
	for _, caseResult := range report.cases {
		result = append(result, cloneAuxiliaryInstanceResult(caseResult))
	}
	return result
}

// Case returns the result at index and reports whether it exists.
func (report AuxiliaryInstanceReport) Case(index int) (AuxiliaryInstanceResult, bool) {
	if index < 0 || index >= len(report.cases) {
		return AuxiliaryInstanceResult{}, false
	}
	return cloneAuxiliaryInstanceResult(report.cases[index]), true
}

// HeadlineCount returns zero because all auxiliary replay results are
// evidence outside headline conformance.
func (report AuxiliaryInstanceReport) HeadlineCount() int {
	return 0
}

// Execute resolves and executes the validated auxiliary plan sequentially.
// Each schema is parsed through ParseSchemaWithPolicy before its paired
// instance is passed to ValidateInstance.
func (plan AuxiliaryInstancePlan) Execute(ctx context.Context, fsys fs.FS) (AuxiliaryInstanceReport, error) {
	if ctx == nil {
		return AuxiliaryInstanceReport{}, executionErrorFor(instanceExecutionContextCode, "", errors.New("nil execution context"))
	}
	if fsys == nil {
		return AuxiliaryInstanceReport{}, executionErrorFor(instanceExecutionFilesystemCode, "", errors.New("nil pinned filesystem"))
	}
	if len(plan.cases) == 0 {
		return AuxiliaryInstanceReport{}, instancePlanError("", errors.New("cannot execute an empty auxiliary instance plan"))
	}

	report := AuxiliaryInstanceReport{
		policy: NewEffectiveExpectationPolicy(plan.policy.overrides),
		cases:  make([]AuxiliaryInstanceResult, 0, len(plan.cases)),
	}
	resolver := pinnedResolver{fsys: fsys}
	for _, planned := range plan.cases {
		schema, schemaExecution := executeSchemaCase(
			ctx, fsys, resolver, planned.policy, planned.version, planned.schemaCase(),
		)
		caseResult := AuxiliaryInstanceResult{
			caseValue:   cloneAuxiliaryInstanceCase(planned),
			schemaStage: auxiliaryStageFromSchemaExecution(schemaExecution),
			instanceStage: AuxiliaryStageResult{
				actual:          ActualNotExecuted,
				outcome:         OutcomeNotExecuted,
				executionReason: "schema-stage-failed",
			},
		}
		if schemaExecution.actual != ActualValid {
			report.cases = append(report.cases, caseResult)
			continue
		}

		instanceFile, err := fsys.Open(planned.instancePath)
		if err != nil {
			caseResult.instanceStage = AuxiliaryStageResult{
				actual:      ActualUnknown,
				actualClass: goxsd9.FailureResolution,
				outcome:     OutcomeResolutionFailure,
				err:         executionErrorFor(instanceExecutionResourceCode, planned.instancePath, err),
			}
			report.cases = append(report.cases, caseResult)
			continue
		}
		if instanceFile == nil {
			caseResult.instanceStage = AuxiliaryStageResult{
				actual:      ActualUnknown,
				actualClass: goxsd9.FailureInternal,
				outcome:     OutcomeInternalFailure,
				err: executionErrorFor(instanceExecutionResourceCode, planned.instancePath,
					errors.New("pinned filesystem returned a nil instance file")),
			}
			report.cases = append(report.cases, caseResult)
			continue
		}

		instanceErr := goxsd9.ValidateInstance(
			schema, goxsd9.SourceID(planned.instancePath), instanceFile,
		)
		if instanceErr == nil {
			caseResult.instanceStage = AuxiliaryStageResult{
				actual:  ActualValid,
				outcome: compareExpectedValidity(planned.effectiveValidity, ActualValid),
			}
			caseResult.sourceMatch = validityMatches(planned.sourceValidity, ActualValid)
			caseResult.effectiveMatch = validityMatches(planned.effectiveValidity, ActualValid)
			report.cases = append(report.cases, caseResult)
			continue
		}

		diagnostics := parserDiagnostics(instanceErr)
		actual, actualClass, outcome := classifyParserFailure(diagnostics, planned.effectiveValidity)
		caseResult.instanceStage = AuxiliaryStageResult{
			actual:      actual,
			actualClass: actualClass,
			outcome:     outcome,
			diagnostics: append([]goxsd9.Diagnostic(nil), diagnostics...),
			err:         instanceErr,
		}
		caseResult.sourceMatch = validityMatches(planned.sourceValidity, actual)
		caseResult.effectiveMatch = validityMatches(planned.effectiveValidity, actual)
		report.cases = append(report.cases, caseResult)
	}
	return report, nil
}

func (caseValue AuxiliaryInstanceCase) schemaCase() catalogCase {
	return cloneCatalogCase(caseValue.pairedSchema)
}

type schemaExecutionResult struct {
	schema          goxsd9.Schema
	actual          ActualResult
	actualClass     goxsd9.FailureClass
	outcome         Outcome
	diagnostics     []goxsd9.Diagnostic
	err             error
	executionReason string
}

func executeSchemaCase(ctx context.Context, fsys fs.FS, resolver pinnedResolver,
	policy goxsd9.LanguagePolicy, version string, caseValue catalogCase,
) (goxsd9.Schema, schemaExecutionResult) {
	execution := schemaExecutionResult{
		actual:  ActualNotExecuted,
		outcome: OutcomeNotExecuted,
	}
	if len(caseValue.documents) == 0 {
		execution.executionReason = "schema-document-missing"
		return goxsd9.Schema{}, execution
	}

	root, graphResolver, sourceClass, sourceErr := openSchemaGraph(ctx, fsys, resolver, caseValue.documents)
	if sourceErr != nil {
		execution.actual = ActualUnknown
		execution.actualClass = sourceClass
		execution.outcome = executionFailureOutcome(sourceClass)
		execution.err = sourceErr
		return goxsd9.Schema{}, execution
	}

	schema, parseErr := goxsd9.ParseSchemaWithPolicy(root, graphResolver, policy)
	if parseErr == nil {
		execution.schema = schema
		execution.actual = ActualValid
		execution.outcome = compareExpectedValidity(caseExpectedValidity(caseValue, version), ActualValid)
		return schema, execution
	}

	execution.err = parseErr
	execution.diagnostics = parserDiagnostics(parseErr)
	execution.actual, execution.actualClass, execution.outcome = classifyParserFailure(
		execution.diagnostics, caseExpectedValidity(caseValue, version),
	)
	return goxsd9.Schema{}, execution
}

func caseExpectedValidity(caseValue catalogCase, version string) string {
	_, expected, _, _ := caseFacts(caseValue, version)
	return expected
}

func auxiliaryStageFromSchemaExecution(execution schemaExecutionResult) AuxiliaryStageResult {
	return AuxiliaryStageResult{
		actual:          execution.actual,
		actualClass:     execution.actualClass,
		outcome:         execution.outcome,
		diagnostics:     append([]goxsd9.Diagnostic(nil), execution.diagnostics...),
		err:             execution.err,
		executionReason: execution.executionReason,
	}
}

func validityMatches(expected string, actual ActualResult) bool {
	return compareExpectedValidity(expected, actual) == OutcomePass
}

func validateEffectiveExpectationPolicy(policy EffectiveExpectationPolicy, cases []catalogCase) error {
	for index, override := range policy.overrides {
		if err := validateEffectiveExpectationOverride(policy, cases, index, override); err != nil {
			return err
		}
	}
	return nil
}

func validateEffectiveExpectationOverride(policy EffectiveExpectationPolicy, cases []catalogCase,
	index int, override EffectiveExpectationOverride,
) error {
	if err := override.validate(); err != nil {
		return instancePlanError(override.Key.SetPath,
			fmt.Errorf("effective-expectation override %d: %w", index+1, err))
	}
	if previousIndex := duplicateOverrideIndex(policy.overrides, index, override.Key); previousIndex >= 0 {
		return instancePlanError(override.Key.SetPath,
			fmt.Errorf("effective-expectation override %d repeats key from entry %d",
				index+1, previousIndex+1))
	}

	instance, ok := findInstanceByIdentity(cases, override.Key)
	if !ok {
		return instancePlanError(override.Key.SetPath,
			fmt.Errorf("effective-expectation override %d has unknown instance key %+v",
				index+1, override.Key))
	}
	if !isAuxiliaryInstanceProvenance(instance) {
		return instancePlanError(instance.setPath,
			fmt.Errorf("effective-expectation override %d targets invalid instance provenance: %s/%s/%s",
				index+1, instance.setPath, instance.groupName, instance.name))
	}
	version, err := uniqueAuxiliaryInstanceVersion(instance)
	if err != nil {
		return instancePlanError(instance.setPath, err)
	}
	if version != override.Key.Version {
		return instancePlanError(instance.setPath,
			fmt.Errorf("effective-expectation override %d uses XSD version %q, want %q",
				index+1, override.Key.Version, version))
	}
	_, sourceValidity, _, _ := caseFacts(instance, version)
	if sourceValidity != override.SourceValidity {
		return instancePlanError(instance.setPath,
			fmt.Errorf("effective-expectation override %d source validity %q drifted from catalog %q",
				index+1, override.SourceValidity, sourceValidity))
	}
	return nil
}

func duplicateOverrideIndex(overrides []EffectiveExpectationOverride, index int, key InstanceKey) int {
	for previousIndex := 0; previousIndex < index; previousIndex++ {
		if overrides[previousIndex].Key == key {
			return previousIndex
		}
	}
	return -1
}

func (override EffectiveExpectationOverride) validate() error {
	if err := override.Key.validate(); err != nil {
		return err
	}
	if !isKnownOutcome(override.SourceValidity) {
		return fmt.Errorf("source validity %q is not valid or invalid", override.SourceValidity)
	}
	if !isKnownOutcome(override.EffectiveValidity) {
		return fmt.Errorf("effective validity %q is not valid or invalid", override.EffectiveValidity)
	}
	return nil
}

func (key InstanceKey) validate() error {
	if key.SetPath == "" {
		return errors.New("instance key SetPath is required")
	}
	if err := validateSelectorPath(key.SetPath); err != nil {
		return fmt.Errorf("instance key SetPath: %w", err)
	}
	if key.GroupName == "" {
		return errors.New("instance key GroupName is required")
	}
	if err := validateOptionalSelectorText("GroupName", key.GroupName); err != nil {
		return fmt.Errorf("instance key GroupName: %w", err)
	}
	if key.CaseName == "" {
		return errors.New("instance key CaseName is required")
	}
	if err := validateOptionalSelectorText("CaseName", key.CaseName); err != nil {
		return fmt.Errorf("instance key CaseName: %w", err)
	}
	if key.Version != "1.0" && key.Version != "1.1" {
		return fmt.Errorf("instance key Version %q is not an exact XSD version", key.Version)
	}
	return nil
}

func isAuxiliaryInstanceCandidate(caseValue catalogCase) bool {
	return isAuxiliaryInstanceProvenance(caseValue)
}

func isAuxiliaryInstanceProvenance(caseValue catalogCase) bool {
	return caseValue.kind == instanceKind && caseValue.origin == originAuxiliary &&
		caseValue.status == statusAccepted && caseValue.status != statusQueried
}

func findInstanceByIdentity(cases []catalogCase, key InstanceKey) (catalogCase, bool) {
	var found catalogCase
	foundCount := 0
	for _, caseValue := range cases {
		if caseValue.kind != instanceKind || caseValue.setPath != key.SetPath ||
			caseValue.groupName != key.GroupName || caseValue.name != key.CaseName {
			continue
		}
		found = caseValue
		foundCount++
	}
	if foundCount != 1 {
		return catalogCase{}, false
	}
	return found, true
}

func uniqueAuxiliaryInstanceVersion(caseValue catalogCase) (string, error) {
	versions := make([]string, 0, 2)
	for _, version := range [...]string{auxiliaryInstanceVersion10, auxiliaryInstanceVersion11} {
		if caseApplies(caseValue, version) {
			versions = append(versions, version)
		}
	}
	if len(versions) != 1 {
		return "", fmt.Errorf("instance case %q has %d applicable XSD versions, want one", caseValue.name, len(versions))
	}
	return versions[0], nil
}

func (inventory Inventory) pairedAuxiliarySchema(instance catalogCase, version string) (catalogCase, error) {
	var paired catalogCase
	pairedCount := 0
	for _, caseValue := range inventory.cases {
		if caseValue.kind != schemaKind || caseValue.setPath != instance.setPath || caseValue.groupName != instance.groupName {
			continue
		}
		paired = caseValue
		pairedCount++
	}
	if pairedCount == 0 {
		return catalogCase{}, fmt.Errorf("instance case %q has no paired schema case", instance.name)
	}
	if pairedCount != 1 {
		return catalogCase{}, fmt.Errorf("instance case %q has %d paired schema cases, want one", instance.name, pairedCount)
	}
	if paired.origin != originAuxiliary || paired.status != statusAccepted || paired.status == statusQueried {
		return catalogCase{}, fmt.Errorf("instance case %q paired schema has invalid provenance", instance.name)
	}
	if !caseApplies(paired, version) {
		return catalogCase{}, fmt.Errorf("instance case %q paired schema does not apply to XSD version %q", instance.name, version)
	}
	expected, expectedValidity, usable, reasons := caseFacts(paired, version)
	if !usable || len(expected) != 1 || expectedValidity != "valid" {
		return catalogCase{}, fmt.Errorf("instance case %q paired schema is not valid for XSD version %q: expected=%q reasons=%s",
			instance.name, version, expectedValidity, strings.Join(reasons, ","))
	}
	if len(paired.documents) == 0 {
		return catalogCase{}, fmt.Errorf("instance case %q paired schema has no schema document", instance.name)
	}
	return paired, nil
}

func instancePlanError(resourcePath string, cause error) error {
	return catalogError(instancePlanErrorCode, resourcePath, cause)
}

func cloneAuxiliaryInstanceCase(caseValue AuxiliaryInstanceCase) AuxiliaryInstanceCase {
	clone := caseValue
	clone.pairedSchema = cloneCatalogCase(caseValue.pairedSchema)
	return clone
}

func cloneAuxiliaryStageResult(result AuxiliaryStageResult) AuxiliaryStageResult {
	clone := result
	clone.diagnostics = append([]goxsd9.Diagnostic(nil), result.diagnostics...)
	return clone
}

func cloneAuxiliaryInstanceResult(result AuxiliaryInstanceResult) AuxiliaryInstanceResult {
	clone := result
	clone.caseValue = cloneAuxiliaryInstanceCase(result.caseValue)
	clone.schemaStage = cloneAuxiliaryStageResult(result.schemaStage)
	clone.instanceStage = cloneAuxiliaryStageResult(result.instanceStage)
	return clone
}

// Write prints the ordered auxiliary replay report, including both execution
// stages and each stage's diagnostics and causes.
func (report AuxiliaryInstanceReport) Write(w io.Writer) error {
	if w == nil {
		return errors.New("write auxiliary instance report: nil writer")
	}
	if _, err := fmt.Fprintln(w, "W3C XML Schema auxiliary instance replay"); err != nil {
		return fmt.Errorf("write auxiliary instance heading: %w", err)
	}
	if _, err := fmt.Fprintf(w, "cases: %d\nheadline-eligible: %d\n\n", len(report.cases), report.HeadlineCount()); err != nil {
		return fmt.Errorf("write auxiliary instance summary: %w", err)
	}
	for index, result := range report.cases {
		if err := writeAuxiliaryInstanceResult(w, index+1, result); err != nil {
			return err
		}
	}
	return nil
}

func writeAuxiliaryInstanceResult(w io.Writer, index int, result AuxiliaryInstanceResult) error {
	if _, err := fmt.Fprintf(w,
		"case %d set=%q set-name=%q group=%q name=%q origin=%s version=%s policy=%s status=%s schema-path=%q instance-path=%q source-expected=%q effective-expected=%q source-match=%t effective-match=%t headline=%t schema-actual=%s schema-class=%s schema-outcome=%s instance-actual=%s instance-class=%s instance-outcome=%s\n",
		index, result.SetPath(), result.SetName(), result.GroupName(), result.CaseName(), result.Origin(),
		result.Version(), result.Policy(), result.Status(), result.SchemaPath(), result.InstancePath(),
		result.SourceExpectedValidity(), result.EffectiveExpectedValidity(), result.SourceMatch(), result.EffectiveMatch(),
		result.HeadlineEligible(), auxiliaryActual(result.schemaStage.actual), auxiliaryClass(result.schemaStage.actualClass),
		result.schemaStage.outcome, auxiliaryActual(result.instanceStage.actual), auxiliaryClass(result.instanceStage.actualClass),
		result.instanceStage.outcome); err != nil {
		return fmt.Errorf("write auxiliary instance case %d: %w", index, err)
	}
	for documentIndex, document := range result.SchemaPaths() {
		if _, err := fmt.Fprintf(w, "schema-document case=%d index=%d path=%q\n", index, documentIndex+1, document); err != nil {
			return fmt.Errorf("write auxiliary instance case %d schema document %d: %w", index, documentIndex+1, err)
		}
	}
	if err := writeAuxiliaryStage(w, index, "schema", result.schemaStage); err != nil {
		return err
	}
	return writeAuxiliaryStage(w, index, "instance", result.instanceStage)
}

func writeAuxiliaryStage(w io.Writer, caseIndex int, name string, result AuxiliaryStageResult) error {
	for diagnosticIndex, diagnostic := range result.diagnostics {
		cause := ""
		if diagnostic.Unwrap() != nil {
			cause = diagnostic.Unwrap().Error()
		}
		if _, err := fmt.Fprintf(w,
			"%s-diagnostic case=%d index=%d class=%s code=%q loc=%q spec=%q message=%q cause=%q\n",
			name, caseIndex, diagnosticIndex+1, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc().String(),
			diagnostic.SpecRef(), diagnostic.Message(), cause); err != nil {
			return fmt.Errorf("write auxiliary instance case %d %s diagnostic %d: %w",
				caseIndex, name, diagnosticIndex+1, err)
		}
	}
	if result.err != nil {
		if _, err := fmt.Fprintf(w, "%s-error case=%d value=%q\n", name, caseIndex, result.err.Error()); err != nil {
			return fmt.Errorf("write auxiliary instance case %d %s error: %w", caseIndex, name, err)
		}
	}
	if result.executionReason != "" {
		if _, err := fmt.Fprintf(w, "%s-execution-reason case=%d value=%q\n", name, caseIndex, result.executionReason); err != nil {
			return fmt.Errorf("write auxiliary instance case %d %s execution reason: %w", caseIndex, name, err)
		}
	}
	return nil
}

func auxiliaryActual(actual ActualResult) ActualResult {
	if actual == "" {
		return ActualNotExecuted
	}
	return actual
}

func auxiliaryClass(class goxsd9.FailureClass) string {
	if class == "" {
		return "-"
	}
	return string(class)
}
