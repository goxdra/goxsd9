package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/goxdra/goxsd9"
)

type plannedSelector struct {
	selector Selector
	policy   goxsd9.LanguagePolicy
}

type plannedCase struct {
	caseValue catalogCase
	version   string
	policy    goxsd9.LanguagePolicy
}

// ExecutionRequirements declares which per-case outcomes a packet accepts.
// An omitted AllowedOutcomes slice leaves the ordinary per-case reporting
// behavior unchanged; a non-empty slice can require support or resources
// without knowing any feature or catalog name.
type ExecutionRequirements struct {
	AllowedOutcomes []Outcome
}

func (requirements ExecutionRequirements) validate() error {
	for index, outcome := range requirements.AllowedOutcomes {
		if !isKnownExecutionOutcome(outcome) {
			return fmt.Errorf("execution requirement outcome %d is unknown: %q", index+1, outcome)
		}
		for previousIndex := 0; previousIndex < index; previousIndex++ {
			if requirements.AllowedOutcomes[previousIndex] == outcome {
				return fmt.Errorf("execution requirement repeats outcome %q", outcome)
			}
		}
	}
	return nil
}

func isKnownExecutionOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomePass, OutcomeConformanceFailure, OutcomeUnsupported,
		OutcomeResolutionFailure, OutcomeInternalFailure, OutcomeNotExecuted:
		return true
	default:
		return false
	}
}

// SelectionPlan is an immutable ordered plan of schema cases that may use
// more than one catalog edition policy.
type SelectionPlan struct {
	selectors []plannedSelector
	cases     []plannedCase
}

// Selectors returns the validated selectors in their declared order.
func (plan SelectionPlan) Selectors() []Selector {
	selectors := make([]Selector, 0, len(plan.selectors))
	for _, planned := range plan.selectors {
		selectors = append(selectors, planned.selector)
	}
	return selectors
}

// Len returns the number of ordered schema cases in the plan.
func (plan SelectionPlan) Len() int {
	return len(plan.cases)
}

// Plan validates selectors and policies, then assigns matching schema cases
// while walking the inventory's catalog order exactly once. Selectors may
// cover different editions of the same catalog case, but an exact
// set/group/case/version assignment may occur only once.
func (inventory Inventory) Plan(selectors []Selector) (SelectionPlan, error) {
	if len(selectors) == 0 {
		return SelectionPlan{}, catalogError("catalog.plan", "",
			errors.New("schema selection plan requires at least one selector"))
	}

	plannedSelectors, err := validatePlanSelectors(selectors)
	if err != nil {
		return SelectionPlan{}, err
	}

	plannedCases, selectorMatches, err := planCatalogCases(inventory.cases, plannedSelectors)
	if err != nil {
		return SelectionPlan{}, err
	}

	if err := validatePlanSelectorMatches(plannedSelectors, selectorMatches); err != nil {
		return SelectionPlan{}, err
	}
	if len(plannedCases) == 0 {
		return SelectionPlan{}, catalogError("catalog.plan", "",
			errors.New("schema selection plan contains zero schema cases"))
	}

	return SelectionPlan{
		selectors: plannedSelectors,
		cases:     plannedCases,
	}, nil
}

func validatePlanSelectors(selectors []Selector) ([]plannedSelector, error) {
	plannedSelectors := make([]plannedSelector, 0, len(selectors))
	for index, selector := range selectors {
		if err := selector.validate(); err != nil {
			return nil, catalogError("catalog.plan", selector.SetPath,
				fmt.Errorf("selector %d: %w", index+1, err))
		}
		policy, err := LanguagePolicyForVersions([]string{selector.Version})
		if err != nil {
			return nil, err
		}
		plannedSelectors = append(plannedSelectors, plannedSelector{
			selector: selector,
			policy:   policy,
		})
	}
	return plannedSelectors, nil
}

type planCaseMatch struct {
	selectorIndex int
	planned       plannedCase
}

func planCatalogCases(cases []catalogCase, selectors []plannedSelector) ([]plannedCase, []int, error) {
	selectorMatches := make([]int, len(selectors))
	plannedCases := make([]plannedCase, 0)
	for _, caseValue := range cases {
		if caseValue.kind != schemaKind {
			continue
		}
		matches, err := planCaseMatches(caseValue, selectors)
		if err != nil {
			return nil, nil, err
		}
		for _, match := range matches {
			selectorMatches[match.selectorIndex]++
			plannedCases = append(plannedCases, match.planned)
		}
	}
	return plannedCases, selectorMatches, nil
}

func planCaseMatches(caseValue catalogCase, selectors []plannedSelector) ([]planCaseMatch, error) {
	matches := make([]planCaseMatch, 0, 2)
	for selectorIndex, planned := range selectors {
		if !planned.selector.matches(caseValue) || !caseApplies(caseValue, planned.selector.Version) {
			continue
		}
		for _, previous := range matches {
			if selectors[previous.selectorIndex].selector.Version != planned.selector.Version {
				continue
			}
			return nil, duplicatePlanCaseError(caseValue, planned.selector.Version,
				previous.selectorIndex, selectorIndex)
		}
		matches = append(matches, planCaseMatch{
			selectorIndex: selectorIndex,
			planned: plannedCase{
				caseValue: cloneCatalogCase(caseValue),
				version:   planned.selector.Version,
				policy:    planned.policy,
			},
		})
	}
	return matches, nil
}

func validatePlanSelectorMatches(selectors []plannedSelector, matches []int) error {
	for index, planned := range selectors {
		if matches[index] == 0 {
			return missingPlanSelectorError(index, planned.selector)
		}
		if planned.selector.CaseName == "" || matches[index] == 1 {
			continue
		}
		return catalogError("catalog.plan", planned.selector.SetPath,
			fmt.Errorf("selector %d is ambiguous for case %q: planned %d schema cases",
				index+1, planned.selector.CaseName, matches[index]))
	}
	return nil
}

func missingPlanSelectorError(index int, selector Selector) error {
	return catalogError("catalog.plan", selector.SetPath,
		fmt.Errorf("selector %d planned zero schema cases: set=%q group=%q case=%q version=%q",
			index+1, selector.SetPath, selector.GroupName, selector.CaseName, selector.Version))
}

func duplicatePlanCaseError(caseValue catalogCase, version string, first, second int) error {
	return catalogError("catalog.plan", caseValue.setPath,
		fmt.Errorf("schema case set=%q set-name=%q group=%q case=%q version=%q matches selectors %d and %d",
			caseValue.setPath, caseValue.setName, caseValue.groupName, caseValue.name, version,
			first+1, second+1))
}

// Execute resolves and parses the plan's schema graphs solely through fsys.
// Cases execute sequentially in catalog order and retain per-case failures.
func (plan SelectionPlan) Execute(ctx context.Context, fsys fs.FS) (PlanReport, error) {
	return plan.execute(ctx, fsys)
}

// ExecuteWithRequirements executes the plan and rejects any case outcome not
// listed by requirements. The report is retained with the error so callers
// can inspect the exact ordered case that failed the packet requirement.
func (plan SelectionPlan) ExecuteWithRequirements(ctx context.Context, fsys fs.FS,
	requirements ExecutionRequirements,
) (PlanReport, error) {
	if err := requirements.validate(); err != nil {
		return PlanReport{}, executionErrorFor("execution.requirement", "", err)
	}
	report, err := plan.execute(ctx, fsys)
	if err != nil {
		return PlanReport{}, err
	}
	if err := report.Check(requirements); err != nil {
		return report, err
	}
	return report, nil
}

func (plan SelectionPlan) execute(ctx context.Context, fsys fs.FS) (PlanReport, error) {
	if ctx == nil {
		return PlanReport{}, executionErrorFor("execution.context", "", errors.New("nil execution context"))
	}
	if fsys == nil {
		return PlanReport{}, executionErrorFor("execution.filesystem", "", errors.New("nil pinned filesystem"))
	}
	if len(plan.selectors) == 0 || len(plan.cases) == 0 {
		return PlanReport{}, catalogError("catalog.plan", "", errors.New("cannot execute an empty schema selection plan"))
	}

	report := PlanReport{
		selectors: clonePlannedSelectors(plan.selectors),
		cases:     make([]CaseResult, 0, len(plan.cases)),
	}
	resolver := pinnedResolver{fsys: fsys}
	for _, planned := range plan.cases {
		report.cases = append(report.cases,
			executeCase(ctx, fsys, resolver, planned.policy, planned.version, planned.caseValue))
	}
	return report, nil
}

// PlanReport is the deterministic result of a mixed-edition schema execution
// plan.
type PlanReport struct {
	selectors []plannedSelector
	cases     []CaseResult
}

// Selectors returns the plan selectors in their declared order.
func (report PlanReport) Selectors() []Selector {
	selectors := make([]Selector, 0, len(report.selectors))
	for _, planned := range report.selectors {
		selectors = append(selectors, planned.selector)
	}
	return selectors
}

// Len returns the number of ordered case results.
func (report PlanReport) Len() int {
	return len(report.cases)
}

// Cases returns a copy of the ordered case results.
func (report PlanReport) Cases() []CaseResult {
	results := make([]CaseResult, 0, len(report.cases))
	for _, result := range report.cases {
		results = append(results, cloneCaseResult(result))
	}
	return results
}

// Case returns the case result at index and reports whether it exists.
func (report PlanReport) Case(index int) (CaseResult, bool) {
	if index < 0 || index >= len(report.cases) {
		return CaseResult{}, false
	}
	return cloneCaseResult(report.cases[index]), true
}

// HeadlineCount counts cases eligible for headline evidence without
// considering whether their execution outcome passed.
func (report PlanReport) HeadlineCount() int {
	count := 0
	for _, result := range report.cases {
		if result.headline {
			count++
		}
	}
	return count
}

// Check verifies the report against packet-level outcome requirements. It
// preserves the first disallowed case's cause and catalog identity.
func (report PlanReport) Check(requirements ExecutionRequirements) error {
	if err := requirements.validate(); err != nil {
		return executionErrorFor("execution.requirement", "", err)
	}
	if len(requirements.AllowedOutcomes) == 0 {
		return nil
	}
	for _, result := range report.cases {
		if executionOutcomeAllowed(result.outcome, requirements.AllowedOutcomes) {
			continue
		}
		return executionRequirementError(result)
	}
	return nil
}

func executionOutcomeAllowed(outcome Outcome, allowed []Outcome) bool {
	for _, candidate := range allowed {
		if candidate == outcome {
			return true
		}
	}
	return false
}

func executionRequirementError(result CaseResult) error {
	cause := result.Cause()
	if cause == nil {
		cause = errors.New("case did not satisfy the execution requirement")
	}
	return executionErrorFor("execution.requirement", result.SetPath(),
		fmt.Errorf("case set=%q group=%q name=%q version=%q produced disallowed outcome %q: %w",
			result.SetPath(), result.GroupName(), result.CaseName(), result.Version(), result.Outcome(), cause))
}

// Write prints a deterministic mixed-edition report in plan order.
func (report PlanReport) Write(w io.Writer) error {
	if w == nil {
		return errors.New("write execution plan report: nil writer")
	}
	if _, err := fmt.Fprintln(w, "W3C XML Schema bounded schema execution plan"); err != nil {
		return fmt.Errorf("write execution plan heading: %w", err)
	}
	if _, err := fmt.Fprintf(w, "selectors: %d\n", len(report.selectors)); err != nil {
		return fmt.Errorf("write execution plan selector count: %w", err)
	}
	for index, planned := range report.selectors {
		selector := planned.selector
		if _, err := fmt.Fprintf(w, "selector %d set=%q group=%q case=%q version=%s policy=%s\n",
			index+1, selector.SetPath, selector.GroupName, selector.CaseName,
			selector.Version, planned.policy); err != nil {
			return fmt.Errorf("write execution plan selector %d: %w", index+1, err)
		}
	}
	if _, err := fmt.Fprintf(w, "cases: %d\nheadline-eligible: %d\n\n", len(report.cases), report.HeadlineCount()); err != nil {
		return fmt.Errorf("write execution plan summary: %w", err)
	}
	for index, result := range report.cases {
		if err := writeCaseResult(w, index+1, result); err != nil {
			return err
		}
	}
	return nil
}

func clonePlannedSelectors(selectors []plannedSelector) []plannedSelector {
	return append([]plannedSelector(nil), selectors...)
}
