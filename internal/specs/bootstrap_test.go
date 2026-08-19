package specs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestBootstrapPlanOrdersBothVersionsDeterministically(t *testing.T) {
	manifest := bootstrapPlanFixture()
	tests := []struct {
		version string
		want    []string
	}{
		{
			version: "1.0",
			want:    []string{"one-early", "one-late", "shared", "one-entry"},
		},
		{
			version: "1.1",
			want:    []string{"two-datatype", "shared", "two-entry"},
		},
	}
	for _, test := range tests {
		plan, err := manifest.BootstrapPlan(test.version)
		if err != nil {
			t.Fatalf("BootstrapPlan(%q) error = %v", test.version, err)
		}
		if plan.Version() != test.version {
			t.Fatalf("BootstrapPlan(%q).Version() = %q", test.version, plan.Version())
		}
		entries := plan.Entries()
		if got := bootstrapEntryIDs(entries); !slices.Equal(got, test.want) {
			t.Fatalf("BootstrapPlan(%q) IDs = %#v, want %#v", test.version, got, test.want)
		}
		entryCount := 0
		for _, entry := range entries {
			if entry.Entry {
				entryCount++
			}
		}
		if entryCount != 1 || !entries[len(entries)-1].Entry {
			t.Fatalf("BootstrapPlan(%q) entry placement is invalid: %#v", test.version, entries)
		}
	}
}

func TestBootstrapPlanRejectsInvalidVersionAndGraph(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		version  string
		code     string
		message  string
	}{
		{
			name:     "invalid version",
			manifest: bootstrapPlanFixture(),
			version:  "2.0",
			code:     bootstrapVersionCode,
			message:  "unsupported bootstrap XSD version",
		},
		{
			name: "missing dependency",
			manifest: Manifest{BootstrapArtifacts: []BootstrapArtifact{
				bootstrapArtifact("root", []string{"1.0"}, true, []string{"missing"}),
			}},
			version: "1.0",
			code:    bootstrapDependencyCode,
			message: "missing from the manifest",
		},
		{
			name: "out of version dependency",
			manifest: Manifest{BootstrapArtifacts: []BootstrapArtifact{
				bootstrapArtifact("root", []string{"1.0"}, true, []string{"foreign"}),
				bootstrapArtifact("foreign", []string{"1.1"}, false, nil),
			}},
			version: "1.0",
			code:    bootstrapDependencyCode,
			message: "not selected for XSD version 1.0",
		},
		{
			name: "cycle",
			manifest: Manifest{BootstrapArtifacts: []BootstrapArtifact{
				bootstrapArtifact("root", []string{"1.0"}, true, []string{"child"}),
				bootstrapArtifact("child", []string{"1.0"}, false, []string{"root"}),
			}},
			version: "1.0",
			code:    bootstrapCycleCode,
			message: "contain a cycle",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.manifest.BootstrapPlan(test.version)
			if err == nil {
				t.Fatal("BootstrapPlan() error = nil")
			}
			assertErrorCode(t, err, test.code)
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("BootstrapPlan() error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestBootstrapPlanReturnsIndependentEntryCopies(t *testing.T) {
	manifest := bootstrapPlanFixture()
	plan, err := manifest.BootstrapPlan("1.0")
	if err != nil {
		t.Fatalf("BootstrapPlan() error = %v", err)
	}

	entries := plan.Entries()
	mutateBootstrapPlanEntries(entries)
	manifest.BootstrapArtifacts[0].Dependencies[0] = "changed-manifest-dependency"

	assertBootstrapPlanCopies(t, plan.Entries())
}

func mutateBootstrapPlanEntries(entries []Entry) {
	for index := range entries {
		switch entries[index].ID {
		case "one-entry":
			entries[index].Dependencies[0] = "changed-plan-dependency"
		case "shared":
			entries[index].Aliases[0] = "https://changed.example/xml.xsd"
			entries[index].XSDVersions[0] = "changed-version"
		}
	}
}

func assertBootstrapPlanCopies(t *testing.T, entries []Entry) {
	t.Helper()
	for _, entry := range entries {
		switch entry.ID {
		case "one-entry":
			if entry.Dependencies[0] != "one-late" {
				t.Fatalf("plan dependency changed through returned slice: %#v", entry.Dependencies)
			}
		case "shared":
			if entry.Aliases[0] != "https://www.example/xml.xsd" {
				t.Fatalf("plan alias changed through returned slice: %#v", entry.Aliases)
			}
			if entry.XSDVersions[0] != "1.0" {
				t.Fatalf("plan version changed through returned slice: %#v", entry.XSDVersions)
			}
		}
	}
}

func TestGenerateBootstrapReturnsNoPartialDocumentsAndPreservesCause(t *testing.T) {
	leafBody := []byte("<xs:schema id=\"leaf\"/>\n")
	root := bootstrapArtifact("root", []string{"1.0"}, true, []string{"leaf"})
	leaf := bootstrapArtifact("leaf", []string{"1.0"}, false, nil)
	leaf.SHA256 = testDigest(leafBody)
	manifest := Manifest{BootstrapArtifacts: []BootstrapArtifact{root, leaf}}
	cause := errors.New("transport cause")
	transport := &bootstrapResponseTransport{
		bodies:     map[string][]byte{leaf.URL: leafBody},
		failureURL: root.URL,
		failure:    cause,
	}

	documents, err := GenerateBootstrap(context.Background(), &http.Client{Transport: transport}, manifest, "1.0")
	if err == nil {
		t.Fatal("GenerateBootstrap() error = nil")
	}
	if documents != nil {
		t.Fatalf("GenerateBootstrap() documents = %#v, want nil after failure", documents)
	}
	assertErrorCode(t, err, "specs.network.request")
	if !errors.Is(err, cause) {
		t.Fatalf("GenerateBootstrap() error = %v, want transport cause", err)
	}
	wantCalls := []string{leaf.URL, root.URL}
	if !slices.Equal(transport.calls, wantCalls) {
		t.Fatalf("GenerateBootstrap() request order = %#v, want %#v", transport.calls, wantCalls)
	}
}

func TestPinnedBootstrapManifestPlan(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest(%q) error = %v", root, err)
	}

	tests := []struct {
		version string
		ids     []string
		reprs   []string
	}{
		{
			version: "1.0",
			ids:     []string{"xsd10-datatypes-schema", "xml-schema", "xsd10-schema-for-schemas"},
			reprs:   []string{"html-cdata-pre", "xml", "xml"},
		},
		{
			version: "1.1",
			ids:     []string{"xsd11-datatypes-schema", "xml-schema", "xsd11-schema-for-schemas"},
			reprs:   []string{"xml", "xml", "xml"},
		},
	}
	for _, test := range tests {
		assertPinnedBootstrapPlan(t, manifest, test.version, test.ids, test.reprs)
	}
}

func TestGenerateBootstrapRegeneratesByteIdentically(t *testing.T) {
	manifest := bootstrapGenerationManifest()
	bodies := bootstrapGenerationBodies(&manifest)

	for _, version := range []string{"1.0", "1.1"} {
		t.Run(version, func(t *testing.T) {
			first, second, transport := generateBootstrapTwice(t, manifest, bodies, version)
			assertGeneratedDocumentsEqual(t, first, second)
			assertBootstrapRequestOrder(t, manifest, version, transport)
			assertGeneratedRepresentations(t, first, version)
		})
	}
}

func mutateBootstrapGenerationBody(artifact *BootstrapArtifact) []byte {
	content := []byte("<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\" id=\"" + artifact.ID + "\"/>\n")
	if artifact.Representation != "html-cdata-pre" {
		return content
	}
	raw := append([]byte(cdataPrefix), content...)
	return append(raw, []byte(cdataSuffix+"\n")...)
}

func bootstrapGenerationBodies(manifest *Manifest) map[string][]byte {
	bodies := make(map[string][]byte, len(manifest.BootstrapArtifacts))
	for index := range manifest.BootstrapArtifacts {
		artifact := &manifest.BootstrapArtifacts[index]
		raw := mutateBootstrapGenerationBody(artifact)
		artifact.SHA256 = testDigest(raw)
		bodies[artifact.URL] = raw
	}
	return bodies
}

func generateBootstrapTwice(
	t *testing.T,
	manifest Manifest,
	bodies map[string][]byte,
	version string,
) ([]Document, []Document, *bootstrapResponseTransport) {
	t.Helper()
	transport := &bootstrapResponseTransport{bodies: bodies}
	client := &http.Client{Transport: transport}
	first, err := GenerateBootstrap(context.Background(), client, manifest, version)
	if err != nil {
		t.Fatalf("GenerateBootstrap() first error = %v", err)
	}
	second, err := GenerateBootstrap(context.Background(), client, manifest, version)
	if err != nil {
		t.Fatalf("GenerateBootstrap() second error = %v", err)
	}
	return first, second, transport
}

func assertGeneratedDocumentsEqual(t *testing.T, first, second []Document) {
	t.Helper()
	if len(first) != len(second) {
		t.Fatalf("generated document counts = %d and %d", len(first), len(second))
	}
	for index := range first {
		if !bytes.Equal(first[index].Data, second[index].Data) {
			t.Fatalf("generated data %d changed between runs: %q and %q", index, first[index].Data, second[index].Data)
		}
		if !equalIndex(first[index].Index, second[index].Index) {
			t.Fatalf("generated index %d changed between runs", index)
		}
	}
}

func assertPinnedBootstrapPlan(t *testing.T, manifest Manifest, version string, wantIDs, wantRepresentations []string) {
	t.Helper()
	plan, err := manifest.BootstrapPlan(version)
	if err != nil {
		t.Fatalf("BootstrapPlan(%q) error = %v", version, err)
	}
	entries := plan.Entries()
	if got := bootstrapEntryIDs(entries); !slices.Equal(got, wantIDs) {
		t.Fatalf("pinned BootstrapPlan(%q) IDs = %#v, want %#v", version, got, wantIDs)
	}
	for index, entry := range entries {
		if entry.Representation != wantRepresentations[index] {
			t.Fatalf("pinned entry %q representation = %q, want %q", entry.ID, entry.Representation, wantRepresentations[index])
		}
	}
	if !slices.Equal(entries[1].Aliases, []string{"http://www.w3.org/2001/xml.xsd"}) {
		t.Fatalf("pinned xml-schema aliases = %#v", entries[1].Aliases)
	}
	if !entries[len(entries)-1].Entry {
		t.Fatalf("pinned entry artifact is not last: %#v", entries)
	}
}

func assertBootstrapRequestOrder(t *testing.T, manifest Manifest, version string, transport *bootstrapResponseTransport) {
	t.Helper()
	plan, err := manifest.BootstrapPlan(version)
	if err != nil {
		t.Fatalf("BootstrapPlan() error = %v", err)
	}
	entries := plan.Entries()
	wantCalls := make([]string, 0, len(entries)*2)
	for _, entry := range entries {
		wantCalls = append(wantCalls, entry.URL)
	}
	for _, entry := range entries {
		wantCalls = append(wantCalls, entry.URL)
	}
	if !slices.Equal(transport.calls, wantCalls) {
		t.Fatalf("generation request order = %#v, want %#v", transport.calls, wantCalls)
	}
}

func assertGeneratedRepresentations(t *testing.T, documents []Document, version string) {
	t.Helper()
	sawXML := false
	sawHTMLCDATA := false
	for _, document := range documents {
		switch document.Entry.Representation {
		case "xml":
			sawXML = true
		case "html-cdata-pre":
			sawHTMLCDATA = true
			if bytes.HasPrefix(document.Data, []byte(cdataPrefix)) || bytes.HasSuffix(document.Data, []byte(cdataSuffix)) {
				t.Fatalf("html-cdata-pre conversion left wrapper in %q", document.Entry.ID)
			}
		}
	}
	if !sawXML {
		t.Fatal("generation did not exercise XML")
	}
	if version == "1.0" && !sawHTMLCDATA {
		t.Fatal("XSD 1.0 generation did not exercise html-cdata-pre")
	}
}

func bootstrapPlanFixture() Manifest {
	return Manifest{BootstrapArtifacts: []BootstrapArtifact{
		bootstrapArtifact("one-entry", []string{"1.0"}, true, []string{"one-late", "shared"}),
		bootstrapArtifact("one-late", []string{"1.0"}, false, []string{"one-early"}),
		bootstrapArtifact("one-early", []string{"1.0"}, false, nil),
		bootstrapArtifact("two-entry", []string{"1.1"}, true, []string{"two-datatype", "shared"}),
		bootstrapArtifact("two-datatype", []string{"1.1"}, false, nil),
		{
			ID:             "shared",
			Title:          "Shared bootstrap artifact",
			URL:            "https://www.w3.org/TR/2020/shared.xsd",
			SHA256:         strings.Repeat("a", 64),
			XSDVersions:    []string{"1.0", "1.1"},
			Representation: "xml",
			Aliases:        []string{"https://www.example/xml.xsd"},
		},
	}}
}

func bootstrapGenerationManifest() Manifest {
	xsd10Entry := bootstrapArtifact("xsd10-entry", []string{"1.0"}, true, []string{"xsd10-datatypes", "xml-schema"})
	xsd10Datatypes := bootstrapArtifact("xsd10-datatypes", []string{"1.0"}, false, nil)
	xsd10Datatypes.Representation = "html-cdata-pre"
	xsd11Entry := bootstrapArtifact("xsd11-entry", []string{"1.1"}, true, []string{"xsd11-datatypes", "xml-schema"})
	xsd11Datatypes := bootstrapArtifact("xsd11-datatypes", []string{"1.1"}, false, nil)
	xmlSchema := bootstrapArtifact("xml-schema", []string{"1.0", "1.1"}, false, nil)
	xmlSchema.Aliases = []string{"http://www.w3.org/2001/xml.xsd"}
	return Manifest{BootstrapArtifacts: []BootstrapArtifact{
		xsd10Entry,
		xsd10Datatypes,
		xsd11Entry,
		xsd11Datatypes,
		xmlSchema,
	}}
}

func bootstrapArtifact(id string, versions []string, entry bool, dependencies []string) BootstrapArtifact {
	return BootstrapArtifact{
		ID:             id,
		Title:          "Bootstrap artifact " + id,
		URL:            "https://www.w3.org/TR/2020/" + id + ".xsd",
		SHA256:         strings.Repeat("a", 64),
		XSDVersions:    versions,
		Representation: "xml",
		Entry:          entry,
		Dependencies:   dependencies,
	}
}

func bootstrapEntryIDs(entries []Entry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

type bootstrapResponseTransport struct {
	bodies     map[string][]byte
	calls      []string
	failureURL string
	failure    error
}

func (transport *bootstrapResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	url := request.URL.String()
	transport.calls = append(transport.calls, url)
	if url == transport.failureURL {
		return nil, transport.failure
	}
	body, ok := transport.bodies[url]
	if !ok {
		return nil, fmt.Errorf("no bootstrap fixture response for %s", url)
	}
	return testResponse(http.StatusOK, append([]byte(nil), body...)), nil
}
