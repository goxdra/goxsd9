package conformance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goxdra/goxsd9"
)

// The selected rows retain their catalog identities and expected outcomes;
// every schema is parsed before its one instance is validated.
//
//nolint:gocognit // Keep the six pinned rows and their exact replay outcomes together.
func TestAuxiliaryPrecisionDecimalListUnionPacket(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	inventory, err := ReadDirectory(root)
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	key := InstanceKey{SetPath: precisionDecimalSaxonSetPath, GroupName: "pdecimal006", CaseName: "pdecimal006.n2.xml", Version: auxiliaryInstanceVersion11}
	policy := NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{Key: key, SourceValidity: "invalid", EffectiveValidity: "valid"}})
	plan, err := inventory.PlanAuxiliaryInstances(policy)
	if err != nil {
		t.Fatalf("PlanAuxiliaryInstances: %v", err)
	}
	selected := AuxiliaryInstancePlan{policy: plan.policy}
	var sourceValid, sourceInvalid int
	for index, row := range plan.cases {
		if index != 46 && index != 47 && index != 48 && index != 49 && index != 52 && index != 53 {
			continue
		}
		want := precisionDecimalAuxiliaryInstanceLedger[index]
		if want.owner != 218 || row.InstancePath() != want.instancePath || row.SchemaPath() != want.schemaPath || row.GroupName() != want.groupName || row.SourceExpectedValidity() != want.outcome {
			t.Fatalf("row %d drifted from #218 ledger: %#v", index+1, row)
		}
		selected.cases = append(selected.cases, row)
		if want.outcome == "valid" {
			sourceValid++
		}
		if want.outcome == "invalid" {
			sourceInvalid++
		}
	}
	if selected.Len() != 6 || sourceValid != 4 || sourceInvalid != 2 {
		t.Fatalf("selected %d rows, %d valid, %d invalid; want 6, 4, 2", selected.Len(), sourceValid, sourceInvalid)
	}
	report, err := selected.Execute(context.Background(), os.DirFS(root))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if report.Len() != 6 || report.HeadlineCount() != 0 {
		t.Fatalf("report has %d rows and %d headline rows, want 6 and 0", report.Len(), report.HeadlineCount())
	}
	for index, row := range report.Cases() {
		stage := row.InstanceStage()
		if row.SchemaStage().Actual() != ActualValid || stage.Outcome() != OutcomePass || !row.SourceMatch() || !row.EffectiveMatch() || row.HeadlineEligible() {
			t.Fatalf("row %d %s: schema=%s instance=%s actual=%s class=%s cause=%v", index+1, row.CaseName(), row.SchemaStage().Actual(), stage.Outcome(), stage.Actual(), stage.ActualClass(), stage.Cause())
		}
		if stage.Actual() != actualForExpected(row.SourceExpectedValidity()) {
			t.Fatalf("row %d actual = %s, want %s", index+1, stage.Actual(), row.SourceExpectedValidity())
		}
		if stage.Actual() == ActualValid {
			if len(stage.Diagnostics()) != 0 {
				t.Fatalf("row %d valid with diagnostics %v", index+1, stage.Diagnostics())
			}
			continue
		}
		diagnostics := stage.Diagnostics()
		if len(diagnostics) != 1 || diagnostics[0].Class() != goxsd9.FailureInvalid || diagnostics[0].Loc().IsZero() {
			t.Fatalf("row %d diagnostics = %v, want one located invalid diagnostic", index+1, diagnostics)
		}
		wantCode := goxsd9.InvalidInstanceListCode
		if row.GroupName() == "pdecimal020" {
			wantCode = goxsd9.InvalidInstanceUnionCode
		}
		if diagnostics[0].Code() != wantCode {
			t.Fatalf("row %d code = %s, want %s", index+1, diagnostics[0].Code(), wantCode)
		}
	}
}
