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
		"1.0 schema 2 1 0 1 0 0 0 0 1 1\n",
		"1.0 instance 3 2 1 0 1 1 0 1 1 0\n",
		"headline excludes queried, disputed, and unusable cases",
	} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("inventory output missing %q:\n%s", want, first.String())
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
	var output bytes.Buffer
	if err := inventory.Write(&output); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, want := range []string{
		"queried: 57",
		"disputed: 6",
		"unusable: 211",
		"1.0 schema 14431 11054 3354 23 23 1 0 15 37 14370",
		"1.0 instance 25126 14069 11045 12 34 0 5 124 136 24951",
		"1.1 schema 15418 11793 3604 21 23 1 0 17 38 15356",
		"1.1 instance 26389 14709 11667 13 34 0 0 138 151 26204",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("pinned inventory output missing %q", want)
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
