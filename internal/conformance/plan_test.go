package conformance

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep mixed-order and per-case policy assertions together.
func TestSelectionPlanWalksCatalogOrderWithMixedPolicies(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	fsys.reset()

	plan, err := inventory.Plan([]Selector{
		{Version: "1.0", SetPath: "sets/execution.testSet", CaseName: "invalid"},
		{Version: "1.1", SetPath: "sets/execution.testSet", CaseName: "valid"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got, want := plan.Len(), 2; got != want {
		t.Fatalf("plan length = %d, want %d", got, want)
	}
	if len(fsys.opened) != 0 {
		t.Fatalf("Plan opened sources: %v", fsys.opened)
	}

	report, err := plan.Execute(context.Background(), fsys)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := report.Len(), 2; got != want {
		t.Fatalf("report length = %d, want %d", got, want)
	}
	wantResults := []struct {
		name    string
		version string
		policy  goxsd9.LanguagePolicy
		actual  ActualResult
		outcome Outcome
	}{
		{name: "valid", version: "1.1", policy: goxsd9.Strict11, actual: ActualValid, outcome: OutcomePass},
		{name: "invalid", version: "1.0", policy: goxsd9.Strict10, actual: ActualInvalid, outcome: OutcomePass},
	}
	for index, want := range wantResults {
		result, ok := report.Case(index)
		if !ok {
			t.Fatalf("Case(%d) missing", index)
		}
		if result.CaseName() != want.name || result.Version() != want.version || result.Policy() != want.policy || result.Actual() != want.actual || result.Outcome() != want.outcome {
			t.Fatalf("case %d = name %q version %q policy %q actual %q outcome %q, want %q %q %q %q %q", index,
				result.CaseName(), result.Version(), result.Policy(), result.Actual(), result.Outcome(),
				want.name, want.version, want.policy, want.actual, want.outcome)
		}
	}
	if got, want := fsys.opened, []string{"sets/valid.xsd", "sets/invalid.xsd"}; !equalStrings(got, want) {
		t.Fatalf("opened = %v, want %v", got, want)
	}
	if fsys.closed["sets/instance.xml"] != 0 {
		t.Fatalf("instance source was opened or closed: %d", fsys.closed["sets/instance.xml"])
	}

	var first, second bytes.Buffer
	if err := report.Write(&first); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := report.Write(&second); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("repeated plan reports differ:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	if !strings.Contains(first.String(), "W3C XML Schema bounded schema execution plan\n") ||
		!strings.Contains(first.String(), "cases: 2\nheadline-eligible: 2\n") ||
		!strings.Contains(first.String(), "diagnostic case=2 index=1 ") ||
		!strings.Contains(first.String(), "feature=\"\" loc=") {
		t.Fatalf("plan report summary missing:\n%s", first.String())
	}
}

func TestSelectionPlanRejectsZeroAndDuplicateCasesBeforeExecution(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tests := []struct {
		name      string
		selectors []Selector
	}{
		{name: "zero selectors"},
		{name: "missing schema case", selectors: []Selector{{Version: "1.0", SetPath: "sets/execution.testSet", CaseName: "instance-only"}}},
		{name: "duplicate exact case", selectors: []Selector{
			{Version: "1.0", SetPath: "sets/execution.testSet", CaseName: "valid"},
			{Version: "1.0", SetPath: "sets/execution.testSet", GroupName: "valid-group"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys.reset()
			_, err := inventory.Plan(test.selectors)
			if err == nil {
				t.Fatal("Plan succeeded")
			}
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog.plan" {
				t.Fatalf("Plan error = %v, want catalog.plan", err)
			}
			if len(fsys.opened) != 0 {
				t.Fatalf("Plan opened sources: %v", fsys.opened)
			}
		})
	}
}

func TestSelectionPlanValidatesAllPoliciesBeforePlanningCases(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	fsys.reset()
	_, err = inventory.Plan([]Selector{
		{Version: "1.0", SetPath: "sets/execution.testSet", CaseName: "valid"},
		{Version: "2.0", SetPath: "sets/execution.testSet", CaseName: "invalid"},
	})
	if err == nil {
		t.Fatal("Plan accepted an unknown edition")
	}
	if len(fsys.opened) != 0 {
		t.Fatalf("Plan opened sources: %v", fsys.opened)
	}
}

func TestSelectionPlanZeroValueCannotExecute(t *testing.T) {
	fsys := executionFixtureFS()
	fsys.reset()
	var plan SelectionPlan
	_, err := plan.Execute(context.Background(), fsys)
	if err == nil {
		t.Fatal("zero-value plan executed")
	}
	var catalogErr *CatalogError
	if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog.plan" {
		t.Fatalf("zero-value plan error = %v, want catalog.plan", err)
	}
	if len(fsys.opened) != 0 {
		t.Fatalf("zero-value plan opened sources: %v", fsys.opened)
	}
}

//nolint:gocognit // Keep declared requirements and distinct failure assertions together.
func TestSelectionPlanRequirementsKeepFailureClassesDistinct(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tests := []struct {
		name         string
		caseName     string
		wantOutcome  Outcome
		wantActual   ActualResult
		wantClass    goxsd9.FailureClass
		wantCause    error
		wantUsable   bool
		wantOpened   bool
		requirements ExecutionRequirements
	}{
		{
			name:         "unsupported required",
			caseName:     "unsupported",
			wantOutcome:  OutcomeUnsupported,
			wantActual:   ActualUnknown,
			wantClass:    goxsd9.FailureUnsupported,
			wantCause:    goxsd9.ErrUnsupported,
			wantUsable:   true,
			wantOpened:   true,
			requirements: ExecutionRequirements{AllowedOutcomes: []Outcome{OutcomePass}},
		},
		{
			name:         "resolution required",
			caseName:     "nested-resolution",
			wantOutcome:  OutcomeResolutionFailure,
			wantActual:   ActualUnknown,
			wantClass:    goxsd9.FailureResolution,
			wantCause:    fs.ErrNotExist,
			wantUsable:   true,
			wantOpened:   true,
			requirements: ExecutionRequirements{AllowedOutcomes: []Outcome{OutcomePass, OutcomeUnsupported}},
		},
		{
			name:         "catalog resource required",
			caseName:     "missing-data",
			wantOutcome:  OutcomeNotExecuted,
			wantActual:   ActualNotExecuted,
			wantClass:    "",
			wantUsable:   false,
			wantOpened:   false,
			requirements: ExecutionRequirements{AllowedOutcomes: []Outcome{OutcomePass, OutcomeUnsupported}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys.reset()
			plan, err := inventory.Plan([]Selector{{
				Version: "1.0", SetPath: "sets/execution.testSet", CaseName: test.caseName,
			}})
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			report, err := plan.ExecuteWithRequirements(context.Background(), fsys, test.requirements)
			if err == nil {
				t.Fatal("ExecuteWithRequirements succeeded")
			}
			result, ok := report.Case(0)
			if !ok {
				t.Fatal("required outcome result missing")
			}
			if result.Outcome() != test.wantOutcome || result.Actual() != test.wantActual || result.ActualClass() != test.wantClass || result.Usable() != test.wantUsable {
				t.Fatalf("result = outcome %q actual %q class %q usable %t, want %q %q %q %t",
					result.Outcome(), result.Actual(), result.ActualClass(), result.Usable(),
					test.wantOutcome, test.wantActual, test.wantClass, test.wantUsable)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("requirement error = %v, want cause %v", err, test.wantCause)
			}
			if (len(fsys.opened) != 0) != test.wantOpened {
				t.Fatalf("schema boundary opened %v, want %t: %v", len(fsys.opened) != 0, test.wantOpened, fsys.opened)
			}
		})
	}
}

func TestSelectionPlanRequirementsValidateBeforeSources(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	plan, err := inventory.Plan([]Selector{{
		Version: "1.0", SetPath: "sets/execution.testSet", CaseName: "valid",
	}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	fsys.reset()
	_, err = plan.ExecuteWithRequirements(context.Background(), fsys, ExecutionRequirements{
		AllowedOutcomes: []Outcome{"not-a-real-outcome"},
	})
	if err == nil {
		t.Fatal("ExecuteWithRequirements accepted an unknown required outcome")
	}
	if len(fsys.opened) != 0 {
		t.Fatalf("invalid requirements opened sources: %v", fsys.opened)
	}
}
