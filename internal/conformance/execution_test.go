package conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit,funlen // Keep the independent catalog and parser outcome assertions together.
func TestSelectionExecuteClassifiesValidityAndDiagnostics(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	selection, err := inventory.Select(Selector{Version: "1.0", SetPath: "sets/execution.testSet"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got, want := selection.Policy(), goxsd9.Strict10; got != want {
		t.Fatalf("policy = %q, want %q", got, want)
	}
	fsys.reset()
	report, err := selection.Execute(context.Background(), fsys)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wantNames := []string{
		"valid", "invalid", "mismatch-valid", "mismatch-invalid", "unsupported",
		"nested-resolution", "multi-document", "queried", "disputed-test",
		"disputed-spec", "missing-status", "missing-data",
	}
	if got, want := report.Len(), len(wantNames); got != want {
		t.Fatalf("report length = %d, want %d", got, want)
	}
	wantResults := []struct {
		name           string
		expected       string
		actual         ActualResult
		actualClass    goxsd9.FailureClass
		outcome        Outcome
		usable         bool
		headline       bool
		reason         string
		diagnostic     goxsd9.FailureClass
		wantDiagnostic bool
	}{
		{name: "valid", expected: "valid", actual: ActualValid, outcome: OutcomePass, usable: true, headline: true},
		{name: "invalid", expected: "invalid", actual: ActualInvalid, actualClass: goxsd9.FailureInvalid, outcome: OutcomePass, usable: true, headline: true, diagnostic: goxsd9.FailureInvalid, wantDiagnostic: true},
		{name: "mismatch-valid", expected: "valid", actual: ActualInvalid, actualClass: goxsd9.FailureInvalid, outcome: OutcomeConformanceFailure, usable: true, headline: true, diagnostic: goxsd9.FailureInvalid, wantDiagnostic: true},
		{name: "mismatch-invalid", expected: "invalid", actual: ActualValid, outcome: OutcomeConformanceFailure, usable: true, headline: true},
		{name: "unsupported", expected: "valid", actual: ActualUnknown, actualClass: goxsd9.FailureUnsupported, outcome: OutcomeUnsupported, usable: true, headline: true, diagnostic: goxsd9.FailureUnsupported, wantDiagnostic: true},
		{name: "nested-resolution", expected: "valid", actual: ActualUnknown, actualClass: goxsd9.FailureResolution, outcome: OutcomeResolutionFailure, usable: true, headline: true, diagnostic: goxsd9.FailureResolution, wantDiagnostic: true},
		{name: "multi-document", expected: "valid", actual: ActualValid, outcome: OutcomePass, usable: true, headline: true},
		{name: "queried", expected: "valid", actual: ActualValid, outcome: OutcomePass, usable: true, headline: false},
		{name: "disputed-test", expected: "invalid", actual: ActualInvalid, actualClass: goxsd9.FailureInvalid, outcome: OutcomePass, usable: true, headline: false, diagnostic: goxsd9.FailureInvalid, wantDiagnostic: true},
		{name: "disputed-spec", expected: "invalid", actual: ActualInvalid, actualClass: goxsd9.FailureInvalid, outcome: OutcomePass, usable: true, headline: false, diagnostic: goxsd9.FailureInvalid, wantDiagnostic: true},
		{name: "missing-status", expected: "valid", actual: ActualNotExecuted, outcome: OutcomeNotExecuted, usable: false, headline: false},
		{name: "missing-data", expected: "valid", actual: ActualNotExecuted, outcome: OutcomeNotExecuted, usable: false, headline: false},
	}
	for index, want := range wantResults {
		result, ok := report.Case(index)
		if !ok {
			t.Fatalf("Case(%d) missing", index)
		}
		if result.CaseName() != wantNames[index] || result.CaseName() != want.name {
			t.Fatalf("case %d name = %q, want %q", index, result.CaseName(), want.name)
		}
		if result.ExpectedValidity() != want.expected || result.Actual() != want.actual || result.ActualClass() != want.actualClass || result.Outcome() != want.outcome {
			t.Fatalf("case %q facts = expected %q actual %q class %q outcome %q, want expected %q actual %q class %q outcome %q", result.CaseName(), result.ExpectedValidity(), result.Actual(), result.ActualClass(), result.Outcome(), want.expected, want.actual, want.actualClass, want.outcome)
		}
		if result.Usable() != want.usable || result.HeadlineEligible() != want.headline || result.ExecutionReason() != want.reason {
			t.Fatalf("case %q catalog/execution facts = usable %t headline %t reason %q, want %t %t %q", result.CaseName(), result.Usable(), result.HeadlineEligible(), result.ExecutionReason(), want.usable, want.headline, want.reason)
		}
		diagnostics := result.Diagnostics()
		if want.wantDiagnostic {
			if len(diagnostics) == 0 || diagnostics[0].Class() != want.diagnostic {
				t.Fatalf("case %q diagnostics = %#v, want first class %q", result.CaseName(), diagnostics, want.diagnostic)
			}
			if result.Cause() == nil {
				t.Fatalf("case %q lost parser cause", result.CaseName())
			}
			continue
		}
		if len(diagnostics) != 0 {
			t.Fatalf("case %q diagnostics = %#v, want none", result.CaseName(), diagnostics)
		}
	}
	for index, wantStatus := range []catalogStatus{
		statusAccepted, statusAccepted, statusAccepted, statusAccepted, statusAccepted,
		statusAccepted, statusAccepted, statusQueried, statusDisputedTest, statusDisputedSpec,
		statusMissing, statusAccepted,
	} {
		result, ok := report.Case(index)
		if !ok || result.Origin() != string(originMain) || result.Status() != string(wantStatus) {
			t.Fatalf("case %d origin/status = %q/%q, want %q/%q", index, result.Origin(), result.Status(), originMain, wantStatus)
		}
	}
	if got, want := report.HeadlineCount(), 7; got != want {
		t.Fatalf("headline count = %d, want %d", got, want)
	}
	nested, ok := report.Case(5)
	if !ok || !errors.Is(nested.Cause(), fs.ErrNotExist) {
		t.Fatalf("nested resolution cause = %v, want fs.ErrNotExist", nested.Cause())
	}
	unsupported, ok := report.Case(4)
	if !ok || !errors.Is(unsupported.Cause(), goxsd9.ErrUnsupported) {
		t.Fatalf("unsupported cause = %v, want goxsd9.ErrUnsupported", unsupported.Cause())
	}
	assertExecutionSources(t, fsys,
		[]string{
			"sets/valid.xsd", "sets/invalid.xsd", "sets/invalid.xsd", "sets/valid.xsd",
			"sets/unsupported.xsd", "sets/nested/root.xsd", "sets/nested/child/child.xsd",
			"sets/first.xsd", "sets/second.xsd", "sets/valid.xsd", "sets/invalid.xsd", "sets/invalid.xsd",
		},
		[]string{
			"sets/valid.xsd", "sets/invalid.xsd", "sets/invalid.xsd", "sets/valid.xsd",
			"sets/unsupported.xsd", "sets/nested/root.xsd", "sets/nested/child/child.xsd",
			"sets/first.xsd", "sets/second.xsd", "sets/valid.xsd", "sets/invalid.xsd", "sets/invalid.xsd",
		},
	)
}

//nolint:gocognit // Keep both strict editions and their expected/actual comparisons together.
func TestSelectionUsesStrictPolicyForBothCatalogEditions(t *testing.T) {
	for _, test := range []struct {
		version string
		policy  goxsd9.LanguagePolicy
	}{
		{version: "1.0", policy: goxsd9.Strict10},
		{version: "1.1", policy: goxsd9.Strict11},
	} {
		t.Run(test.version, func(t *testing.T) {
			fsys := executionFixtureFS()
			inventory, err := Read(fsys)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			for _, caseName := range []string{"valid", "invalid"} {
				selection, err := inventory.Select(Selector{Version: test.version, SetPath: "sets/execution.testSet", CaseName: caseName})
				if err != nil {
					t.Fatalf("Select %s: %v", caseName, err)
				}
				if selection.Policy() != test.policy {
					t.Fatalf("%s policy = %q, want %q", caseName, selection.Policy(), test.policy)
				}
				fsys.reset()
				report, err := selection.Execute(context.Background(), fsys)
				if err != nil {
					t.Fatalf("Execute %s: %v", caseName, err)
				}
				result, ok := report.Case(0)
				if !ok {
					t.Fatalf("Case %s missing", caseName)
				}
				if caseName == "valid" && (result.Actual() != ActualValid || result.Outcome() != OutcomePass) {
					t.Fatalf("valid result = %q/%q, want valid/pass", result.Actual(), result.Outcome())
				}
				if caseName == "invalid" && (result.Actual() != ActualInvalid || result.Outcome() != OutcomePass) {
					t.Fatalf("invalid result = %q/%q, want invalid/pass", result.Actual(), result.Outcome())
				}
			}
		})
	}
}

func TestSelectionReportsAreDeterministicAcrossRepeatedRuns(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	selection, err := inventory.Select(Selector{Version: "1.0", SetPath: "sets/execution.testSet"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	outputs := make([]string, 0, 2)
	for range 2 {
		fsys.reset()
		report, err := selection.Execute(context.Background(), fsys)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		var output bytes.Buffer
		if err := report.Write(&output); err != nil {
			t.Fatalf("Write: %v", err)
		}
		outputs = append(outputs, output.String())
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("repeated reports differ:\nfirst:\n%s\nsecond:\n%s", outputs[0], outputs[1])
	}
	for index, want := range []string{"valid", "invalid", "mismatch-valid", "mismatch-invalid", "unsupported", "nested-resolution", "multi-document"} {
		result, ok := selectionCaseFromReport(outputs[0], index, want)
		if !ok || result == "" {
			t.Fatalf("report case %d does not preserve catalog order for %q", index, want)
		}
	}
}

func selectionCaseFromReport(output string, index int, want string) (string, bool) {
	needle := fmt.Sprintf("case %d ", index+1)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, needle) {
			return line, strings.Contains(line, `name="`+want+`"`)
		}
	}
	return "", false
}

func TestSelectionRejectsMalformedAndUnknownSelectorsBeforeSourceOpen(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	selectors := []Selector{
		{},
		{Version: "1.0"},
		{Version: "2.0", SetPath: "sets/execution.testSet"},
		{Version: "1.0", SetPath: "../outside"},
		{Version: "1.0", CaseName: "instance-only"},
	}
	for _, selector := range selectors {
		fsys.reset()
		if _, err := inventory.Select(selector); err == nil {
			t.Fatalf("Select(%+v) succeeded", selector)
		}
		if len(fsys.opened) != 0 {
			t.Fatalf("Select(%+v) opened sources: %v", selector, fsys.opened)
		}
	}
}

//nolint:gocognit // Keep catalog, parser, source-order, closure, and report assertions together.
func TestSelectionExecutesMultipleSchemaDocumentsAsOneOrderedGraph(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	selection, err := inventory.Select(Selector{
		Version:  "1.0",
		SetPath:  "sets/execution.testSet",
		CaseName: "multi-document",
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	fsys.reset()
	report, err := selection.Execute(context.Background(), fsys)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	result, ok := report.Case(0)
	if !ok {
		t.Fatal("multi-document result missing")
	}
	if got, want := result.Documents(), []string{"sets/first.xsd", "sets/second.xsd"}; !equalStrings(got, want) {
		t.Fatalf("documents = %v, want %v", got, want)
	}
	if result.Usable() != true || result.HeadlineEligible() != true {
		t.Fatalf("catalog facts = usable %t headline %t, want true true", result.Usable(), result.HeadlineEligible())
	}
	if result.Actual() != ActualValid || result.Outcome() != OutcomePass || result.ExecutionReason() != "" {
		t.Fatalf("execution facts = actual %q outcome %q reason %q, want valid/pass/empty", result.Actual(), result.Outcome(), result.ExecutionReason())
	}
	if got, want := fsys.opened, []string{"sets/first.xsd", "sets/second.xsd"}; !equalStrings(got, want) {
		t.Fatalf("multi-document case opened sources: %v, want %v", got, want)
	}
	for _, path := range []string{"sets/first.xsd", "sets/second.xsd"} {
		if got := fsys.closed[path]; got != 1 {
			t.Fatalf("multi-document case closed[%q] = %d, want 1", path, got)
		}
	}

	var first, second bytes.Buffer
	if err := report.Write(&first); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := report.Write(&second); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if first.String() != second.String() || !strings.Contains(first.String(), `document case=1 index=1 path="sets/first.xsd"`) || !strings.Contains(first.String(), `document case=1 index=2 path="sets/second.xsd"`) {
		t.Fatalf("multi-document report is not deterministic or complete:\n%s", first.String())
	}
}

func TestSelectionPreservesAuxiliaryStatusAndHeadlineFacts(t *testing.T) {
	fsys := executionFixtureFS()
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	selection, err := inventory.Select(Selector{Version: "1.1", CaseName: "aux-valid"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	fsys.reset()
	report, err := selection.Execute(context.Background(), fsys)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	result, ok := report.Case(0)
	if !ok {
		t.Fatal("auxiliary result missing")
	}
	if result.Origin() != string(originAuxiliary) || result.Status() != string(statusAccepted) || !result.Usable() || result.Outcome() != OutcomePass || result.HeadlineEligible() {
		t.Fatalf("auxiliary facts = origin %q status %q usable %t outcome %q headline %t", result.Origin(), result.Status(), result.Usable(), result.Outcome(), result.HeadlineEligible())
	}
}

func TestClassifyParserFailureUsesInternalFallback(t *testing.T) {
	actual, class, outcome := classifyParserFailure(nil, "valid")
	if actual != ActualUnknown || class != goxsd9.FailureInternal || outcome != OutcomeInternalFailure {
		t.Fatalf("fallback = actual %q class %q outcome %q, want unknown/internal/internal-failure", actual, class, outcome)
	}

	fsys := executionFixtureFS()
	fsys.nilFiles["sets/valid.xsd"] = true
	inventory, err := Read(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	selection, err := inventory.Select(Selector{Version: "1.0", SetPath: "sets/execution.testSet", CaseName: "valid"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	report, err := selection.Execute(context.Background(), fsys)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	result, ok := report.Case(0)
	if !ok || result.Actual() != ActualUnknown || result.ActualClass() != goxsd9.FailureInternal || result.Outcome() != OutcomeInternalFailure || result.Cause() == nil {
		t.Fatalf("nil-file result = %#v, want internal failure with cause", result)
	}
}

func TestPinnedCatalogBoundedSchemaSmoke(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	resources := os.DirFS(root)
	inventory, err := Read(resources)
	if err != nil {
		t.Fatalf("Read pinned catalog: %v", err)
	}
	selection, err := inventory.Select(Selector{
		Version:  "1.0",
		SetPath:  "nistMeta/NISTXMLSchemaDatatypes.testSet",
		CaseName: "NISTSchema-SV-IV-atomic-decimal-minExclusive-1",
	})
	if err != nil {
		t.Fatalf("Select pinned case: %v", err)
	}
	report, err := selection.Execute(context.Background(), resources)
	if err != nil {
		t.Fatalf("Execute pinned case: %v", err)
	}
	result, ok := report.Case(0)
	if !ok || result.Outcome() != OutcomePass || result.Actual() != ActualValid {
		t.Fatalf("pinned result = actual %q outcome %q, want valid/pass", result.Actual(), result.Outcome())
	}
}

func TestPinnedCatalogExecutesSubsgroupMultiDocumentCase(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	resources := os.DirFS(root)
	inventory, err := Read(resources)
	if err != nil {
		t.Fatalf("Read pinned catalog: %v", err)
	}
	selection, err := inventory.Select(Selector{
		Version:  "1.1",
		SetPath:  "saxonMeta/Subsgroup.testSet",
		CaseName: "subsgroup003a.xsd",
	})
	if err != nil {
		t.Fatalf("Select pinned Subsgroup case: %v", err)
	}
	report, err := selection.Execute(context.Background(), resources)
	if err != nil {
		t.Fatalf("Execute pinned Subsgroup case: %v", err)
	}
	result, ok := report.Case(0)
	if !ok {
		t.Fatal("pinned Subsgroup result missing")
	}
	if got, want := result.Documents(), []string{
		"saxonData/Subsgroup/subsgroup003a.xsd",
		"saxonData/Subsgroup/subsgroup003b.xsd",
		"saxonData/Subsgroup/subsgroup003c.xsd",
	}; !equalStrings(got, want) {
		t.Fatalf("pinned Subsgroup documents = %v, want %v", got, want)
	}
	if result.Actual() == ActualNotExecuted || result.Outcome() == OutcomeNotExecuted {
		t.Fatalf("pinned Subsgroup case was not executed: actual %q outcome %q reason %q", result.Actual(), result.Outcome(), result.ExecutionReason())
	}
	if result.ActualClass() != goxsd9.FailureUnsupported || result.Outcome() != OutcomeUnsupported || !errors.Is(result.Cause(), goxsd9.ErrUnsupported) {
		t.Fatalf("pinned Subsgroup facts = actual %q class %q outcome %q cause %v, want unsupported parser outcome", result.Actual(), result.ActualClass(), result.Outcome(), result.Cause())
	}
}

func assertExecutionSources(t *testing.T, fsys *executionTrackingFS, wantOpened, wantClosed []string) {
	t.Helper()
	if !equalStrings(fsys.opened, wantOpened) {
		t.Fatalf("opened = %v, want %v", fsys.opened, wantOpened)
	}
	for _, path := range wantClosed {
		if got, want := fsys.closed[path], countStrings(wantClosed, path); got != want {
			t.Fatalf("closed[%q] = %d, want %d", path, got, want)
		}
	}
	if fsys.closed["sets/instance.xml"] != 0 {
		t.Fatalf("instance source was closed despite not being executed: %d", fsys.closed["sets/instance.xml"])
	}
}

func countStrings(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

type executionTrackingFS struct {
	files    fstest.MapFS
	opened   []string
	closed   map[string]int
	nilFiles map[string]bool
}

func (fsys *executionTrackingFS) Open(name string) (fs.File, error) {
	if fsys.nilFiles[name] {
		return nil, nil
	}
	file, err := fsys.files.Open(name)
	if err != nil {
		return nil, err
	}
	fsys.opened = append(fsys.opened, name)
	return &executionTrackedFile{File: file, path: name, owner: fsys}, nil
}

func (fsys *executionTrackingFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(fsys.files, name)
}

func (fsys *executionTrackingFS) reset() {
	fsys.opened = nil
	fsys.closed = make(map[string]int)
}

type executionTrackedFile struct {
	fs.File
	path   string
	owner  *executionTrackingFS
	closed bool
}

func (file *executionTrackedFile) Close() error {
	if file.closed {
		return nil
	}
	file.closed = true
	file.owner.closed[file.path]++
	return file.File.Close()
}

func executionFixtureFS() *executionTrackingFS {
	return &executionTrackingFS{
		files: fstest.MapFS{
			"suite.xml":                   &fstest.MapFile{Data: []byte(executionSuite)},
			"extra-suite.xml":             &fstest.MapFile{Data: []byte(executionExtraSuite)},
			"sets/execution.testSet":      &fstest.MapFile{Data: []byte(executionTestSet)},
			"sets/auxiliary.testSet":      &fstest.MapFile{Data: []byte(executionAuxiliaryTestSet)},
			"sets/valid.xsd":              &fstest.MapFile{Data: []byte(executionValidSchema)},
			"sets/invalid.xsd":            &fstest.MapFile{Data: []byte(executionInvalidSchema)},
			"sets/unsupported.xsd":        &fstest.MapFile{Data: []byte(executionUnsupportedSchema)},
			"sets/nested/root.xsd":        &fstest.MapFile{Data: []byte(executionNestedRootSchema)},
			"sets/nested/child/child.xsd": &fstest.MapFile{Data: []byte(executionNestedChildSchema)},
			"sets/first.xsd":              &fstest.MapFile{Data: []byte(executionMultiFirstSchema)},
			"sets/second.xsd":             &fstest.MapFile{Data: []byte(executionMultiSecondSchema)},
			"sets/instance.xml":           &fstest.MapFile{Data: []byte("<instance/>")},
		},
		closed:   make(map[string]int),
		nilFiles: make(map[string]bool),
	}
}

const executionSuite = `<?xml version="1.0"?>
<ts:testSuite xmlns:ts="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="execution" releaseDate="2026-01-01" schemaVersion="fixture">
  <ts:testSetRef xlink:href="sets/execution.testSet"/>
</ts:testSuite>
`

const executionExtraSuite = `<?xml version="1.0"?>
<testSuite xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="execution-extra" releaseDate="2026-01-01" schemaVersion="fixture">
  <testSetRef xlink:href="sets/auxiliary.testSet"/>
</testSuite>
`

const executionTestSet = `<?xml version="1.0"?>
<testSet xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="execution-set" contributor="fixture">
  <testGroup name="valid-group">
    <schemaTest name="valid">
      <schemaDocument xlink:href="valid.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="invalid-group">
    <schemaTest name="invalid">
      <schemaDocument xlink:href="invalid.xsd"/>
      <expected validity="invalid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="mismatch-valid-group">
    <schemaTest name="mismatch-valid">
      <schemaDocument xlink:href="invalid.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="mismatch-invalid-group">
    <schemaTest name="mismatch-invalid">
      <schemaDocument xlink:href="valid.xsd"/>
      <expected validity="invalid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="unsupported-group">
    <schemaTest name="unsupported">
      <schemaDocument xlink:href="unsupported.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="nested-resolution-group">
    <schemaTest name="nested-resolution">
      <schemaDocument xlink:href="nested/root.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="multi-document-group">
    <schemaTest name="multi-document">
      <schemaDocument xlink:href="first.xsd"/>
      <schemaDocument xlink:href="second.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="queried-group">
    <schemaTest name="queried">
      <schemaDocument xlink:href="valid.xsd"/>
      <expected validity="valid"/>
      <current status="queried" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="disputed-test-group">
    <schemaTest name="disputed-test">
      <schemaDocument xlink:href="invalid.xsd"/>
      <expected validity="invalid"/>
      <current status="disputed-test" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="disputed-spec-group">
    <schemaTest name="disputed-spec">
      <schemaDocument xlink:href="invalid.xsd"/>
      <expected validity="invalid"/>
      <current status="disputed-spec" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="missing-status-group">
    <schemaTest name="missing-status">
      <schemaDocument xlink:href="valid.xsd"/>
      <expected validity="valid"/>
    </schemaTest>
  </testGroup>
  <testGroup name="missing-data-group">
    <schemaTest name="missing-data">
      <schemaDocument xlink:href="missing.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="instance-group">
    <instanceTest name="instance-only">
      <instanceDocument xlink:href="instance.xml"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </instanceTest>
  </testGroup>
</testSet>
`

const executionAuxiliaryTestSet = `<?xml version="1.0"?>
<testSet xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="auxiliary-set" contributor="auxiliary" version="1.1">
  <testGroup name="auxiliary-group">
    <schemaTest name="aux-valid">
      <schemaDocument xlink:href="valid.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
</testSet>
`

const executionValidSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`

const executionInvalidSchema = `<not-schema/>`

const executionUnsupportedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:unknown/></xs:schema>`

const executionNestedRootSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:include schemaLocation="child/child.xsd"/></xs:schema>`

const executionNestedChildSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:include schemaLocation="../missing.xsd"/></xs:schema>`

const executionMultiFirstSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:multi:first"><xs:element name="first"/></xs:schema>`

const executionMultiSecondSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:multi:second"><xs:import namespace="urn:multi:first" schemaLocation="first.xsd"/><xs:element name="second"/></xs:schema>`
