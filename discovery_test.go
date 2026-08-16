package goxsd9

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

type discoveryContextKey struct{}

type discoveryCall struct {
	contextSource string
	namespaceURN  string
	location      string
}

type discoveryFixture struct {
	id       SourceID
	contents string
	closeErr error
}

type discoveryResolver struct {
	fixtures map[string]discoveryFixture
	failures map[string]error
	calls    []discoveryCall
	opened   []*discoveryReader
}

func (resolver *discoveryResolver) Resolve(
	ctx context.Context,
	namespaceURN, schemaLocation string,
) (ResolvedSource, error) {
	contextSource := ""
	if value, ok := ctx.Value(discoveryContextKey{}).(string); ok {
		contextSource = value
	}
	resolver.calls = append(resolver.calls, discoveryCall{
		contextSource: contextSource,
		namespaceURN:  namespaceURN,
		location:      schemaLocation,
	})
	if err, ok := resolver.failures[schemaLocation]; ok {
		return ResolvedSource{}, err
	}
	fixture, ok := resolver.fixtures[schemaLocation]
	if !ok {
		return ResolvedSource{}, fmt.Errorf("no fixture for %q", schemaLocation)
	}
	reader := &discoveryReader{
		data:     []byte(fixture.contents),
		closeErr: fixture.closeErr,
	}
	resolver.opened = append(resolver.opened, reader)
	childContext := context.WithValue(ctx, discoveryContextKey{}, string(fixture.id))
	return NewResolvedSource(childContext, fixture.id, reader)
}

type discoveryReader struct {
	data       []byte
	offset     int
	closeErr   error
	closeCount int
}

func (reader *discoveryReader) Read(buffer []byte) (int, error) {
	if reader.offset == len(reader.data) {
		return 0, io.EOF
	}
	n := copy(buffer, reader.data[reader.offset:])
	reader.offset += n
	return n, nil
}

func (reader *discoveryReader) Close() error {
	reader.closeCount++
	return reader.closeErr
}

func TestDiscoverSyntaxDocumentsUsesSequentialIdentityQueue(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">` +
		`<xs:include schemaLocation="a.xsd"/>` +
		`<xs:import namespace="urn:b" schemaLocation="b.xsd"/>` +
		`<xs:include schemaLocation="a.xsd"/>` +
		`</xs:schema>`
	rootReader := &discoveryReader{data: []byte(rootContents)}
	rootContext := context.WithValue(context.Background(), discoveryContextKey{}, "root")
	root, err := NewResolvedSource(rootContext, "root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}

	resolver := &discoveryResolver{
		fixtures: map[string]discoveryFixture{
			"a.xsd":    {id: "a", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:a"><xs:include schemaLocation="root.xsd"/></xs:schema>`},
			"b.xsd":    {id: "b", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import namespace="urn:root" schemaLocation="root.xsd"/></xs:schema>`},
			"root.xsd": {id: "root", contents: rootContents},
		},
	}

	documents, err := discoverSyntaxDocuments(root, resolver)
	if err != nil {
		t.Fatalf("discoverSyntaxDocuments: %v", err)
	}
	if got, want := len(documents), 3; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	for index, want := range []SourceID{"root", "a", "b"} {
		if got := documents[index].source; got != want {
			t.Fatalf("document %d source = %q, want %q", index, got, want)
		}
	}

	wantCalls := []discoveryCall{
		{contextSource: "root", namespaceURN: "", location: "a.xsd"},
		{contextSource: "root", namespaceURN: "urn:b", location: "b.xsd"},
		{contextSource: "root", namespaceURN: "", location: "a.xsd"},
		{contextSource: "a", namespaceURN: "", location: "root.xsd"},
		{contextSource: "b", namespaceURN: "urn:root", location: "root.xsd"},
	}
	if len(resolver.calls) != len(wantCalls) {
		t.Fatalf("resolver call count = %d, want %d: %#v", len(resolver.calls), len(wantCalls), resolver.calls)
	}
	for index, want := range wantCalls {
		if got := resolver.calls[index]; got != want {
			t.Errorf("resolver call %d = %#v, want %#v", index, got, want)
		}
	}
	if rootReader.closeCount != 1 {
		t.Errorf("root close count = %d, want 1", rootReader.closeCount)
	}
	for index, reader := range resolver.opened {
		if reader.closeCount != 1 {
			t.Errorf("resolved reader %d close count = %d, want 1", index, reader.closeCount)
		}
	}
}

func TestDiscoverSyntaxDocumentsPreservesResolutionFailureAndCleansQueue(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="first.xsd"/><xs:import namespace="urn:bad" schemaLocation="bad.xsd"/></xs:schema>`
	rootReader := &discoveryReader{data: []byte(rootContents)}
	root, err := NewResolvedSource(context.Background(), "root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolveErr := errors.New("resolver offline")
	resolver := &discoveryResolver{
		fixtures: map[string]discoveryFixture{
			"first.xsd": {id: "first", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"/>`},
		},
		failures: map[string]error{"bad.xsd": resolveErr},
	}

	_, err = discoverSyntaxDocuments(root, resolver)
	if err == nil {
		t.Fatal("discoverSyntaxDocuments succeeded for a resolver failure")
	}
	if !errors.Is(err, resolveErr) {
		t.Fatalf("resolution error does not preserve cause: %v", err)
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureResolution; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), SourceResolveCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Loc().Source(), SourceID("root"); got != want {
		t.Fatalf("Loc().Source() = %q, want %q", got, want)
	}
	if diagnostic.Loc().Line() != 1 || diagnostic.Loc().Column() == 0 {
		t.Fatalf("resolution location = %s, want a location in root", diagnostic.Loc())
	}
	if rootReader.closeCount != 1 {
		t.Errorf("root close count = %d, want 1", rootReader.closeCount)
	}
	if len(resolver.opened) != 1 {
		t.Fatalf("opened source count = %d, want 1", len(resolver.opened))
	}
	if resolver.opened[0].closeCount != 1 {
		t.Errorf("pending source close count = %d, want 1", resolver.opened[0].closeCount)
	}
}

func TestDiscoverSyntaxDocumentsReportsPendingCloseFailure(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="first.xsd"/><xs:import schemaLocation="bad.xsd"/></xs:schema>`
	rootReader := &discoveryReader{data: []byte(rootContents)}
	root, err := NewResolvedSource(context.Background(), "root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	closeErr := errors.New("pending close failed")
	resolver := &discoveryResolver{
		fixtures: map[string]discoveryFixture{
			"first.xsd": {id: "first", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"/>`, closeErr: closeErr},
		},
		failures: map[string]error{"bad.xsd": errors.New("resolver offline")},
	}

	_, err = discoverSyntaxDocuments(root, resolver)
	if err == nil {
		t.Fatal("discoverSyntaxDocuments succeeded despite a pending close failure")
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("close error is not preserved: %v", err)
	}
	diagnostics := syntaxDiagnostics(err)
	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count = %d, want %d: %#v", got, want, diagnostics)
	}
	if got, want := diagnostics[1].Code(), SourceCloseCode; got != want {
		t.Fatalf("cleanup diagnostic code = %q, want %q", got, want)
	}
}

func TestDiscoverSyntaxDocumentsRejectsIncludeWithoutLocation(t *testing.T) {
	rootReader := &discoveryReader{data: []byte(`<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include/></xs:schema>`)}
	root, err := NewResolvedSource(context.Background(), "root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}

	_, err = discoverSyntaxDocuments(root, nil)
	if err == nil {
		t.Fatal("discoverSyntaxDocuments accepted an include without schemaLocation")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), MissingSchemaLocationCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if rootReader.closeCount != 1 {
		t.Errorf("root close count = %d, want 1", rootReader.closeCount)
	}
}

func TestDiscoverSyntaxDocumentsPassesEmptyImportLocation(t *testing.T) {
	rootReader := &discoveryReader{data: []byte(`<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import namespace="urn:b"/></xs:schema>`)}
	root, err := NewResolvedSource(context.Background(), "root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{
		fixtures: map[string]discoveryFixture{
			"": {id: "b", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"/>`},
		},
	}

	_, err = discoverSyntaxDocuments(root, resolver)
	if err != nil {
		t.Fatalf("discoverSyntaxDocuments: %v", err)
	}
	if len(resolver.calls) != 1 {
		t.Fatalf("resolver call count = %d, want 1", len(resolver.calls))
	}
	if got, want := resolver.calls[0].location, ""; got != want {
		t.Fatalf("schema location = %q, want empty lexical location", got)
	}
	if got, want := resolver.calls[0].namespaceURN, "urn:b"; got != want {
		t.Fatalf("namespace URN = %q, want %q", got, want)
	}
}
