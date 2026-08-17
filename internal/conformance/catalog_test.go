package conformance

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const fixtureNamespace = testSuiteNamespace

type originCaseExpectation struct {
	setPath string
	name    string
	origin  catalogOrigin
	status  catalogStatus
	outcome string
	usable  bool
}

func TestReadDeduplicatesCatalogRootsAndReportsIndependentDimensions(t *testing.T) {
	inventory, err := Read(fixtureFS(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got, want := len(inventory.setPaths), 1; got != want {
		t.Fatalf("set count = %d, want %d", got, want)
	}
	if got, want := len(inventory.cases), 5; got != want {
		t.Fatalf("case count = %d, want %d", got, want)
	}

	var first, second bytes.Buffer
	if err := inventory.Write(&first); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := inventory.Write(&second); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("Write is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	for _, want := range []string{
		"test-sets: 1\n",
		"cases: 5\n",
		"main-cases: 5\n",
		"auxiliary-cases: 0\n",
		"origin version kind cases valid invalid other submitted accepted stable queried disputed-test disputed-spec status-missing unusable headline",
		"main 1.0 schema 2 1 0 1 0 2 0 0 0 0 0 1 1\n",
		"main 1.0 instance 3 2 1 0 0 0 0 1 1 0 1 1 0\n",
		"Outcome, status, usability, and origin columns are independent",
	} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("inventory output missing %q:\n%s", want, first.String())
		}
	}
}

func TestReadPreservesRootOriginOrderAndMainPrecedence(t *testing.T) {
	inventory, err := Read(originFixtureFS(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	wantPaths := []string{"sets/main.testSet", "sets/shared.testSet", "sets/aux.testSet"}
	assertSetPathOrder(t, inventory.setPaths, wantPaths)
	wantCases := []originCaseExpectation{
		{setPath: "sets/main.testSet", name: "main-schema", origin: originMain, status: statusAccepted, outcome: "valid"},
		{setPath: "sets/shared.testSet", name: "shared-instance", origin: originMain, status: statusStable, outcome: "invalid"},
		{setPath: "sets/aux.testSet", name: "aux-submitted", origin: originAuxiliary, status: statusSubmitted, outcome: "valid"},
		{setPath: "sets/aux.testSet", name: "aux-accepted", origin: originAuxiliary, status: statusAccepted, outcome: "invalid"},
		{setPath: "sets/aux.testSet", name: "aux-stable", origin: originAuxiliary, status: statusStable, outcome: "valid"},
		{setPath: "sets/aux.testSet", name: "aux-unusable", origin: originAuxiliary, status: statusAccepted, outcome: "invalid", usable: true},
	}
	assertOriginCases(t, inventory.cases, wantCases)
	mainCases, auxiliaryCases := inventory.originCaseCounts()
	if mainCases != 2 || auxiliaryCases != 4 {
		t.Fatalf("origin case counts = %d main, %d auxiliary; want 2, 4", mainCases, auxiliaryCases)
	}
	output := inventoryOutput(t, inventory)
	assertOutputContains(t, output, []string{
		"main-cases: 2\n",
		"auxiliary-cases: 4\n",
		"main 1.1 schema 1 1 0 0 0 1 0 0 0 0 0 0 1\n",
		"main 1.1 instance 1 0 1 0 0 0 1 0 0 0 0 0 1\n",
		"auxiliary 1.1 schema 2 1 1 0 1 1 0 0 0 0 0 0 0\n",
		"auxiliary 1.1 instance 2 1 1 0 0 1 1 0 0 0 0 1 0\n",
	})
}

func assertSetPathOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("set paths = %v, want %v", got, want)
	}
	for index, path := range want {
		if got[index] != path {
			t.Fatalf("set path %d = %q, want %q", index, got[index], path)
		}
	}
}

func assertOriginCases(t *testing.T, got []catalogCase, want []originCaseExpectation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("case count = %d, want %d", len(got), len(want))
	}
	for index, expected := range want {
		assertOriginCase(t, index, got[index], expected)
	}
}

func assertOriginCase(t *testing.T, index int, got catalogCase, want originCaseExpectation) {
	t.Helper()
	if got.setPath != want.setPath || got.name != want.name {
		t.Fatalf("case %d = %s/%s, want %s/%s", index, got.setPath, got.name, want.setPath, want.name)
	}
	if got.origin != want.origin {
		t.Fatalf("case %s origin = %s, want %s", got.name, got.origin, want.origin)
	}
	if got.status != want.status {
		t.Fatalf("case %s status = %s, want %s", got.name, got.status, want.status)
	}
	outcome, ok := got.outcome("1.1")
	if !ok || outcome != want.outcome {
		t.Fatalf("case %s outcome = %q, %t; want %q, true", got.name, outcome, ok, want.outcome)
	}
	if usable := got.isUnusable("1.1"); usable != want.usable {
		t.Fatalf("case %s unusable = %t, want %t", got.name, usable, want.usable)
	}
}

func inventoryOutput(t *testing.T, inventory Inventory) string {
	t.Helper()
	var output bytes.Buffer
	if err := inventory.Write(&output); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return output.String()
}

func assertOutputContains(t *testing.T, output string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("inventory output missing %q:\n%s", want, output)
		}
	}
}

func TestReadMarksMissingDataUnusable(t *testing.T) {
	files := fixtureFS(t)
	delete(files, "sets/data.xml")
	inventory, err := Read(files)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var output bytes.Buffer
	if err := inventory.Write(&output); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(output.String(), "unusable: 4\n") {
		t.Fatalf("missing data was not classified as unusable:\n%s", output.String())
	}
}

func TestPinnedCatalogBaseline(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	inventory, err := ReadDirectory(root)
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	if got, want := len(inventory.setPaths), 95; got != want {
		t.Fatalf("pinned test-set count = %d, want %d", got, want)
	}
	if got, want := len(inventory.cases), 41858; got != want {
		t.Fatalf("pinned case count = %d, want %d", got, want)
	}
	mainCases, auxiliaryCases := inventory.originCaseCounts()
	if mainCases != 41735 || auxiliaryCases != 123 {
		t.Fatalf("pinned origin case counts = %d main, %d auxiliary; want 41735 main, 123 auxiliary", mainCases, auxiliaryCases)
	}
	assertPinnedSharedSetIsMain(t, inventory.cases)
	auxiliary, saxonica, ibm := pinnedAuxiliaryCaseCounts(t, inventory.cases)
	if auxiliary != 123 || saxonica != 71 || ibm != 52 {
		t.Fatalf("auxiliary precisionDecimal cases = %d (Saxonica %d, IBM %d), want 123 (71, 52)", auxiliary, saxonica, ibm)
	}
	assertAuxiliaryHeadlinesZero(t, inventory.rows())
	output := inventoryOutput(t, inventory)
	assertOutputContains(t, output, []string{
		"queried: 57",
		"disputed: 6",
		"unusable: 211",
		"main-cases: 41735",
		"auxiliary-cases: 123",
		"main 1.0 schema 14430 11054 3353 23 2 13664 725 23 1 0 15 37 14369",
		"main 1.0 instance 25126 14069 11045 12 10 24839 114 34 0 5 124 136 24951",
		"main 1.1 schema 15365 11768 3576 21 2 14597 725 23 1 0 17 38 15303",
		"main 1.1 instance 26320 14685 11622 13 10 26020 118 34 0 0 138 151 26135",
		"auxiliary 1.0 schema 1 0 1 0 0 1 0 0 0 0 0 0 0",
		"auxiliary 1.1 schema 53 25 28 0 0 53 0 0 0 0 0 0 0",
		"auxiliary 1.1 instance 69 24 45 0 0 69 0 0 0 0 0 0 0",
	})
}

func assertPinnedSharedSetIsMain(t *testing.T, cases []catalogCase) {
	t.Helper()
	shared := 0
	for _, caseValue := range cases {
		if caseValue.setPath != "common/introspection.testSet" {
			continue
		}
		shared++
		if caseValue.origin != originMain {
			t.Fatalf("shared introspection case %s origin = %s, want main", caseValue.name, caseValue.origin)
		}
	}
	if shared == 0 {
		t.Fatal("pinned shared introspection test set has no cases")
	}
}

func pinnedAuxiliaryCaseCounts(t *testing.T, cases []catalogCase) (auxiliary, saxonica, ibm int) {
	t.Helper()
	for _, caseValue := range cases {
		if caseValue.origin != originAuxiliary {
			continue
		}
		auxiliary++
		switch caseValue.setPath {
		case "saxonMeta/PDecimal.testSet":
			saxonica++
		case "ibmMeta/precisionDecimal.testSet":
			ibm++
		default:
			t.Fatalf("unexpected auxiliary case path %q", caseValue.setPath)
		}
		assertPinnedAuxiliaryCase(t, caseValue)
	}
	return auxiliary, saxonica, ibm
}

func assertPinnedAuxiliaryCase(t *testing.T, caseValue catalogCase) {
	t.Helper()
	if caseValue.status != statusAccepted {
		t.Fatalf("auxiliary case %s status = %s, want accepted", caseValue.name, caseValue.status)
	}
	if caseValue.isGloballyUnusable() {
		t.Fatalf("auxiliary case %s is unexpectedly unusable", caseValue.name)
	}
	for _, version := range []string{"1.0", "1.1"} {
		if !caseApplies(caseValue, version) {
			continue
		}
		if _, ok := caseValue.outcome(version); ok {
			return
		}
	}
	t.Fatalf("auxiliary case %s has no expected outcome", caseValue.name)
}

func assertAuxiliaryHeadlinesZero(t *testing.T, rows []inventoryRow) {
	t.Helper()
	for _, row := range rows {
		if row.origin == originAuxiliary && row.headline != 0 {
			t.Fatalf("auxiliary %s %s headline = %d, want 0", row.version, row.kind, row.headline)
		}
	}
}

func TestReadRejectsTerminalDecoderErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "trailing malformed XML", data: []byte(fixtureSuite + "<!--")},
		{name: "unclosed root", data: []byte(strings.TrimSuffix(strings.TrimSpace(fixtureSuite), "</ts:testSuite>"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := fixtureFS(t)
			files["suite.xml"] = &fstest.MapFile{Data: test.data}
			_, err := Read(files)
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) {
				t.Fatalf("Read error = %v, want CatalogError", err)
			}
			if catalogErr.Code != "catalog.decode" {
				t.Fatalf("CatalogError.Code = %q, want catalog.decode", catalogErr.Code)
			}
		})
	}
}

func TestReadRejectsChangedStructureAndMissingTestSet(t *testing.T) {
	tests := []struct {
		name string
		fs   func(*testing.T) fs.FS
		code string
	}{
		{
			name: "unknown child",
			fs: func(t *testing.T) fs.FS {
				files := fixtureFS(t)
				files["suite.xml"] = &fstest.MapFile{Data: []byte(strings.Replace(
					fixtureSuite, "<ts:testSetRef", "<ts:unknown/><ts:testSetRef", 1))}
				return files
			},
			code: "catalog.structure",
		},
		{
			name: "missing test set",
			fs: func(t *testing.T) fs.FS {
				files := fixtureFS(t)
				delete(files, "sets/one.testSet")
				return files
			},
			code: "catalog.read",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Read(test.fs(t))
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) {
				t.Fatalf("Read error = %v, want CatalogError", err)
			}
			if catalogErr.Code != test.code {
				t.Fatalf("CatalogError.Code = %q, want %q", catalogErr.Code, test.code)
			}
		})
	}
}

func TestReadRejectsCatalogOrderChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(fstest.MapFS)
	}{
		{
			name: "suite annotation after reference",
			mutate: func(files fstest.MapFS) {
				files["suite.xml"] = &fstest.MapFile{Data: []byte(strings.Replace(
					fixtureSuite,
					"  <ts:testSetRef xlink:href=\"sets/one.testSet\"/>\n",
					"  <ts:testSetRef xlink:href=\"sets/one.testSet\"/>\n  <ts:annotation/>\n",
					1))}
			},
		},
		{
			name: "test set annotation after group",
			mutate: func(files fstest.MapFS) {
				files["sets/one.testSet"] = &fstest.MapFile{Data: []byte(strings.Replace(
					fixtureTestSet, "  </testGroup>\n", "  </testGroup>\n  <annotation/>\n", 1))}
			},
		},
		{
			name: "case expected after current",
			mutate: func(files fstest.MapFS) {
				files["sets/one.testSet"] = &fstest.MapFile{Data: []byte(strings.Replace(
					fixtureTestSet,
					"      <current status=\"accepted\" date=\"2026-01-01\"/>\n",
					"      <current status=\"accepted\" date=\"2026-01-01\"/>\n      <expected validity=\"valid\"/>\n",
					1))}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := fixtureFS(t)
			test.mutate(files)
			_, err := Read(files)
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) {
				t.Fatalf("Read error = %v, want CatalogError", err)
			}
			if catalogErr.Code != "catalog.structure" {
				t.Fatalf("CatalogError.Code = %q, want catalog.structure", catalogErr.Code)
			}
		})
	}
}

func TestVersionApplicabilitySeparatesParentAndExpected(t *testing.T) {
	for _, test := range []struct {
		name string
		got  bool
		want bool
	}{
		{name: "unscoped parent 1.0", got: parentApplies(nil, "1.0"), want: true},
		{name: "one version parent excludes other", got: parentApplies([]string{"1.0"}, "1.1"), want: false},
		{name: "feature parent remains conditional", got: parentApplies([]string{"Unicode_4.0.0"}, "1.0"), want: true},
		{name: "expected inherits parent", got: expectedApplies(expectation{}, []string{"1.0"}, "1.0"), want: true},
		{name: "expected does not escape parent", got: expectedApplies(expectation{explicit: true, versions: []string{"1.0"}}, []string{"1.1"}, "1.0"), want: false},
		{name: "expected feature conjunction is conditional", got: expectedApplies(expectation{explicit: true, versions: []string{"1.0", "1.0-1e"}}, nil, "1.0"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %t, want %t", test.got, test.want)
			}
		})
	}
}

func TestResolveReferenceRejectsEscapes(t *testing.T) {
	for _, href := range []string{"../../outside.xml", "/absolute.xml", "https://example.test/catalog.xml", "data.xml#part"} {
		if _, err := resolveReference("sets/one.testSet", href); err == nil {
			t.Fatalf("resolveReference(%q) accepted invalid reference", href)
		}
	}
}

func fixtureFS(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"suite.xml":        &fstest.MapFile{Data: []byte(fixtureSuite)},
		"extra-suite.xml":  &fstest.MapFile{Data: []byte(fixtureExtraSuite)},
		"sets/one.testSet": &fstest.MapFile{Data: []byte(fixtureTestSet)},
		"sets/data.xsd":    &fstest.MapFile{Data: []byte("<schema/>")},
		"sets/data.xml":    &fstest.MapFile{Data: []byte("<document/>")},
	}
}

func originFixtureFS(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"suite.xml":           &fstest.MapFile{Data: []byte(originFixtureSuite)},
		"extra-suite.xml":     &fstest.MapFile{Data: []byte(originFixtureExtraSuite)},
		"sets/main.testSet":   &fstest.MapFile{Data: []byte(originFixtureMainSet)},
		"sets/shared.testSet": &fstest.MapFile{Data: []byte(originFixtureSharedSet)},
		"sets/aux.testSet":    &fstest.MapFile{Data: []byte(originFixtureAuxiliarySet)},
		"sets/main.xsd":       &fstest.MapFile{Data: []byte("<schema/>")},
		"sets/shared.xml":     &fstest.MapFile{Data: []byte("<document/>")},
		"sets/aux.xsd":        &fstest.MapFile{Data: []byte("<schema/>")},
		"sets/aux.xml":        &fstest.MapFile{Data: []byte("<document/>")},
	}
}

const fixtureSuite = `<?xml version="1.0"?>
<ts:testSuite xmlns:ts="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="fixture" releaseDate="2026-01-01" schemaVersion="fixture">
  <ts:annotation><ts:documentation>fixture</ts:documentation></ts:annotation>
  <ts:testSetRef xlink:href="sets/one.testSet"/>
</ts:testSuite>
`

const fixtureExtraSuite = `<?xml version="1.0"?>
<testSuite xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="extra" releaseDate="2026-01-01" schemaVersion="fixture">
  <testSetRef xlink:href="sets/one.testSet"/>
</testSuite>
`

const fixtureTestSet = `<?xml version="1.0"?>
<testSet xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="fixture-set" contributor="fixture">
  <annotation><documentation>fixture</documentation></annotation>
  <testGroup name="fixture-group">
    <schemaTest name="valid-schema">
      <schemaDocument xlink:href="data.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
    <instanceTest name="queried-instance">
      <instanceDocument xlink:href="data.xml"/>
      <expected validity="invalid"/>
      <current status="queried" date="2026-01-01"/>
    </instanceTest>
    <instanceTest name="disputed-instance">
      <instanceDocument xlink:href="data.xml"/>
      <expected validity="valid"/>
      <current status="disputed-test" date="2026-01-01"/>
    </instanceTest>
    <instanceTest name="missing-status">
      <instanceDocument xlink:href="data.xml"/>
      <expected validity="valid"/>
    </instanceTest>
  </testGroup>
  <testGroup name="ambiguous-group">
    <schemaTest name="ambiguous-schema">
      <schemaDocument xlink:href="data.xsd"/>
      <expected validity="valid"/>
      <expected version="1.0" validity="invalid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
</testSet>
`

const originFixtureSuite = `<?xml version="1.0"?>
<ts:testSuite xmlns:ts="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="main" releaseDate="2026-01-01" schemaVersion="fixture">
  <ts:testSetRef xlink:href="sets/main.testSet"/>
  <ts:testSetRef xlink:href="sets/shared.testSet"/>
</ts:testSuite>
`

const originFixtureExtraSuite = `<?xml version="1.0"?>
<testSuite xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="auxiliary" releaseDate="2026-01-01" schemaVersion="fixture">
  <testSetRef xlink:href="sets/aux.testSet"/>
  <testSetRef xlink:href="sets/shared.testSet"/>
</testSuite>
`

const originFixtureMainSet = `<?xml version="1.0"?>
<testSet xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="main-set" contributor="main">
  <testGroup name="main-group">
    <schemaTest name="main-schema">
      <schemaDocument xlink:href="main.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
</testSet>
`

const originFixtureSharedSet = `<?xml version="1.0"?>
<testSet xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="shared-set" contributor="shared">
  <testGroup name="shared-group">
    <instanceTest name="shared-instance">
      <instanceDocument xlink:href="shared.xml"/>
      <expected validity="invalid"/>
      <current status="stable" date="2026-01-01"/>
    </instanceTest>
  </testGroup>
</testSet>
`

const originFixtureAuxiliarySet = `<?xml version="1.0"?>
<testSet xmlns="` + fixtureNamespace + `" xmlns:xlink="` + xlinkNamespace + `" name="auxiliary-set" contributor="auxiliary" version="1.1">
  <testGroup name="submitted-group">
    <schemaTest name="aux-submitted">
      <schemaDocument xlink:href="aux.xsd"/>
      <expected validity="valid"/>
      <current status="submitted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="accepted-group">
    <schemaTest name="aux-accepted">
      <schemaDocument xlink:href="aux.xsd"/>
      <expected validity="invalid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
  <testGroup name="stable-group">
    <instanceTest name="aux-stable">
      <instanceDocument xlink:href="aux.xml"/>
      <expected validity="valid"/>
      <current status="stable" date="2026-01-01"/>
    </instanceTest>
  </testGroup>
  <testGroup name="unusable-group">
    <instanceTest name="aux-unusable">
      <instanceDocument xlink:href="missing.xml"/>
      <expected validity="invalid"/>
      <current status="accepted" date="2026-01-01"/>
    </instanceTest>
  </testGroup>
</testSet>
`
