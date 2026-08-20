package goxsd9_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const parseTestXSDNamespace = "http://www.w3.org/2001/XMLSchema"

const parseTestRootDocument = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.0">
  <xs:include schemaLocation="a-location"/>
  <xs:import namespace="urn:b" schemaLocation="b-location"/>
  <xs:include schemaLocation="a-location"/>
  <xs:element name="rootElement"/>
  <xs:simpleType name="rootType">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="4"/>
      <xs:fractionDigits value="2"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

const parseTestADocument = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root">
  <xs:include schemaLocation="root-location"/>
  <xs:element name="aElement"/>
</xs:schema>`

const parseTestBDocument = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:b" version="1.1">
  <xs:import namespace="urn:root" schemaLocation="root-location"/>
  <xs:attribute name="bAttribute"/>
  <xs:simpleType name="bType">
    <xs:restriction base="xs:decimal">
      <xs:fractionDigits value="2"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

type parseTestContextKey struct{}

type parseTestCall struct {
	contextSource string
	namespaceURN  string
	location      string
}

type parseTestReader struct {
	reader     *strings.Reader
	readCount  int
	closeCount int
}

func newParseTestReader(contents string) *parseTestReader {
	return &parseTestReader{reader: strings.NewReader(contents)}
}

func (reader *parseTestReader) Read(buffer []byte) (int, error) {
	reader.readCount++
	return reader.reader.Read(buffer)
}

func (reader *parseTestReader) Close() error {
	reader.closeCount++
	return nil
}

type parseTestOpenedSource struct {
	id     goxsd9.SourceID
	reader *parseTestReader
}

type parseTestResolver struct {
	calls  []parseTestCall
	opened []parseTestOpenedSource
}

func (resolver *parseTestResolver) Resolve(
	ctx context.Context,
	namespaceURN, schemaLocation string,
) (goxsd9.ResolvedSource, error) {
	contextSource := ""
	if ctx != nil {
		if value, ok := ctx.Value(parseTestContextKey{}).(string); ok {
			contextSource = value
		}
	}
	resolver.calls = append(resolver.calls, parseTestCall{
		contextSource: contextSource,
		namespaceURN:  namespaceURN,
		location:      schemaLocation,
	})

	id, contents, ok := parseTestFixture(schemaLocation)
	if !ok {
		return goxsd9.ResolvedSource{}, fmt.Errorf("no fixture for %q", schemaLocation)
	}
	reader := newParseTestReader(contents)
	resolver.opened = append(resolver.opened, parseTestOpenedSource{id: id, reader: reader})
	childContext := context.WithValue(ctx, parseTestContextKey{}, string(id))
	return goxsd9.NewResolvedSource(childContext, id, reader)
}

func parseTestFixture(location string) (goxsd9.SourceID, string, bool) {
	switch location {
	case "a-location":
		return "opaque-a", parseTestADocument, true
	case "b-location":
		return "opaque-b", parseTestBDocument, true
	case "root-location":
		return "opaque-root", parseTestRootDocument, true
	default:
		return "", "", false
	}
}

func TestParseSchemaBuildsDeterministicImmutableGraph(t *testing.T) {
	rootReader := newParseTestReader(parseTestRootDocument)
	rootContext := context.WithValue(context.Background(), parseTestContextKey{}, "root")
	root, err := goxsd9.NewResolvedSource(rootContext, "opaque-root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &parseTestResolver{}

	schema, err := goxsd9.ParseSchema(root, resolver)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	assertParseTestDocuments(t, schema)
	assertParseTestComponents(t, schema)
	assertParseTestResolverCalls(t, resolver)
	assertParseTestSourcesClosed(t, rootReader, resolver)
	assertParseTestMixedVersions(t, schema)
	assertParseTestCopies(t, schema)
}

func assertParseTestDocuments(t *testing.T, schema goxsd9.Schema) {
	t.Helper()
	wantSources := []goxsd9.SourceID{"opaque-root", "opaque-a", "opaque-b"}
	wantNamespaces := []string{"urn:root", "urn:root", "urn:b"}
	documents := schema.Documents()
	if len(documents) != len(wantSources) {
		t.Fatalf("document count = %d, want %d", len(documents), len(wantSources))
	}
	for index := range wantSources {
		if got := documents[index].Source(); got != wantSources[index] {
			t.Errorf("document %d source = %q, want %q", index, got, wantSources[index])
		}
		if got := documents[index].TargetNamespace(); got != wantNamespaces[index] {
			t.Errorf("document %d target namespace = %q, want %q", index, got, wantNamespaces[index])
		}
	}
}

func assertParseTestComponents(t *testing.T, schema goxsd9.Schema) {
	t.Helper()
	want := []struct {
		source goxsd9.SourceID
		kind   goxsd9.ComponentKind
		name   goxsd9.QName
	}{
		{source: "opaque-root", kind: goxsd9.ComponentKindElementDeclaration, name: parseTestQName(t, "urn:root", "rootElement")},
		{source: "opaque-root", kind: goxsd9.ComponentKindSimpleTypeDefinition, name: parseTestQName(t, "urn:root", "rootType")},
		{source: "opaque-a", kind: goxsd9.ComponentKindElementDeclaration, name: parseTestQName(t, "urn:root", "aElement")},
		{source: "opaque-b", kind: goxsd9.ComponentKindAttributeDeclaration, name: parseTestQName(t, "urn:b", "bAttribute")},
		{source: "opaque-b", kind: goxsd9.ComponentKindSimpleTypeDefinition, name: parseTestQName(t, "urn:b", "bType")},
	}
	components := schema.Components()
	if len(components) != len(want) {
		t.Fatalf("component count = %d, want %d", len(components), len(want))
	}
	walked := make([]goxsd9.ComponentID, 0, len(components))
	for index, expected := range want {
		component := components[index]
		if component.Document() != expected.source {
			t.Errorf("component %d source = %q, want %q", index, component.Document(), expected.source)
		}
		if component.Kind() != expected.kind {
			t.Errorf("component %d kind = %q, want %q", index, component.Kind(), expected.kind)
		}
		if component.Name() != expected.name {
			t.Errorf("component %d name = %q, want %q", index, component.Name(), expected.name)
		}
		if component.Loc().Source() != expected.source {
			t.Errorf("component %d location source = %q, want %q", index, component.Loc().Source(), expected.source)
		}
		walked = append(walked, component.ID())
	}

	visited := make([]goxsd9.ComponentID, 0, len(components))
	if err := schema.Walk(func(component goxsd9.Component) error {
		visited = append(visited, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !reflect.DeepEqual(visited, walked) {
		t.Fatalf("walk IDs = %#v, want %#v", visited, walked)
	}
	rootName := parseTestQName(t, "urn:root", "rootElement")
	if found := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName); len(found) != 1 || found[0].ID() != components[0].ID() {
		t.Fatalf("FindKind(rootElement) = %#v, want component 0", found)
	}
}

func assertParseTestResolverCalls(t *testing.T, resolver *parseTestResolver) {
	t.Helper()
	want := []parseTestCall{
		{contextSource: "root", location: "a-location"},
		{contextSource: "root", namespaceURN: "urn:b", location: "b-location"},
		{contextSource: "root", location: "a-location"},
		{contextSource: "opaque-a", location: "root-location"},
		{contextSource: "opaque-b", namespaceURN: "urn:root", location: "root-location"},
	}
	if !reflect.DeepEqual(resolver.calls, want) {
		t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, want)
	}
}

func assertParseTestSourcesClosed(t *testing.T, rootReader *parseTestReader, resolver *parseTestResolver) {
	t.Helper()
	if rootReader.readCount == 0 {
		t.Fatal("root source was not read")
	}
	if rootReader.closeCount != 1 {
		t.Fatalf("root close count = %d, want 1", rootReader.closeCount)
	}
	if len(resolver.opened) != 5 {
		t.Fatalf("resolved source count = %d, want 5", len(resolver.opened))
	}
	wantRead := []bool{true, true, false, false, false}
	for index, opened := range resolver.opened {
		if wantRead[index] && opened.reader.readCount == 0 {
			t.Errorf("resolved source %d (%s) was not read", index, opened.id)
		}
		if !wantRead[index] && opened.reader.readCount != 0 {
			t.Errorf("repeated source %d (%s) was read, want close without read", index, opened.id)
		}
		if opened.reader.closeCount != 1 {
			t.Errorf("resolved source %d (%s) close count = %d, want 1", index, opened.id, opened.reader.closeCount)
		}
	}
}

func assertParseTestMixedVersions(t *testing.T, schema goxsd9.Schema) {
	t.Helper()
	rootType := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, parseTestQName(t, "urn:root", "rootType"))
	childType := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, parseTestQName(t, "urn:b", "bType"))
	if len(rootType) != 1 || len(childType) != 1 {
		t.Fatalf("simple type queries = %d root, %d child; want one each", len(rootType), len(childType))
	}
	rootDefinition, ok := rootType[0].SimpleType()
	if !ok {
		t.Fatal("root simple type view is missing")
	}
	if got, want := rootDefinition.DigitFacets().Version(), goxsd9.XSDVersion10; got != want {
		t.Fatalf("root simple type version = %q, want %q", got, want)
	}
	childDefinition, ok := childType[0].SimpleType()
	if !ok {
		t.Fatal("child simple type view is missing")
	}
	if got, want := childDefinition.DigitFacets().Version(), goxsd9.XSDVersion11; got != want {
		t.Fatalf("child simple type version = %q, want %q", got, want)
	}
}

func assertParseTestCopies(t *testing.T, schema goxsd9.Schema) {
	t.Helper()
	rootName := parseTestQName(t, "urn:root", "rootElement")
	rootID := schema.Components()[0].ID()
	components := schema.Components()
	components[0] = goxsd9.Component{}
	if component, ok := schema.Lookup(rootID); !ok || component.Name() != rootName {
		t.Fatal("mutating Components changed the completed schema")
	}
	documents := schema.Documents()
	documents[0] = goxsd9.SchemaDocument{}
	if got := schema.Documents()[0].Source(); got != "opaque-root" {
		t.Fatalf("mutating Documents changed source to %q", got)
	}
	documentComponents := schema.Documents()[0].Components()
	documentComponents[0] = goxsd9.Component{}
	if got := schema.Documents()[0].Components()[0].Name(); got != rootName {
		t.Fatalf("mutating document Components changed name to %q", got)
	}
	found := schema.Find(rootName)
	found[0] = goxsd9.Component{}
	if got := schema.Find(rootName)[0].Name(); got != rootName {
		t.Fatalf("mutating Find changed name to %q", got)
	}
}

func TestParseSchemaPreservesResolutionCauseWithoutPartialSchema(t *testing.T) {
	rootReader := newParseTestReader(`<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"><xs:include schemaLocation="missing-location"/></xs:schema>`)
	root, err := goxsd9.NewResolvedSource(context.Background(), "opaque-root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolveCause := errors.New("resolver unavailable")
	resolver := parseFailingResolver{cause: resolveCause}

	schema, err := goxsd9.ParseSchema(root, resolver)
	if err == nil {
		t.Fatal("ParseSchema succeeded for a resolution failure")
	}
	if !errors.Is(err, resolveCause) {
		t.Fatalf("resolution cause was not preserved: %v", err)
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T does not contain a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != goxsd9.FailureResolution || diagnostic.Code() != goxsd9.SourceResolveCode {
		t.Fatalf("diagnostic = %s, want source resolution diagnostic", diagnostic)
	}
	if diagnostic.Loc().Source() != "opaque-root" || diagnostic.Loc().Line() != 1 || diagnostic.Loc().Column() == 0 {
		t.Fatalf("diagnostic location = %s, want located root reference", diagnostic.Loc())
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatalf("schema after error = %#v, want zero schema", schema)
	}
	if rootReader.closeCount != 1 {
		t.Fatalf("root close count = %d, want 1", rootReader.closeCount)
	}
}

type parseFailingResolver struct {
	cause error
}

func (resolver parseFailingResolver) Resolve(context.Context, string, string) (goxsd9.ResolvedSource, error) {
	return goxsd9.ResolvedSource{}, resolver.cause
}

func TestParseSchemaValidatesZeroSourceAndLazilyUsesNilResolver(t *testing.T) {
	t.Run("zero source", assertParseTestZeroSource)
	t.Run("unused nil resolver", assertParseTestUnusedNilResolver)
	t.Run("referenced nil resolver", assertParseTestReferencedNilResolver)
}

func assertParseTestZeroSource(t *testing.T) {
	t.Helper()
	schema, err := goxsd9.ParseSchema(goxsd9.ResolvedSource{}, nil)
	if err == nil {
		t.Fatal("ParseSchema accepted a zero ResolvedSource")
	}
	diagnostic := parseTestDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.SourceInvalidCode {
		t.Fatalf("zero-source diagnostic = %s, want invalid source diagnostic", diagnostic)
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("zero-source parse returned a partial schema")
	}
}

func assertParseTestUnusedNilResolver(t *testing.T) {
	t.Helper()
	noReferenceReader := newParseTestReader(`<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"/>`)
	noReferenceRoot, err := goxsd9.NewResolvedSource(context.Background(), "no-reference", noReferenceReader)
	if err != nil {
		t.Fatalf("NewResolvedSource(no reference): %v", err)
	}
	noReferenceSchema, err := goxsd9.ParseSchema(noReferenceRoot, nil)
	if err != nil {
		t.Fatalf("ParseSchema with unused nil resolver: %v", err)
	}
	if len(noReferenceSchema.Documents()) != 1 || noReferenceReader.closeCount != 1 {
		t.Fatalf("nil-resolver no-reference result = %d documents, close count %d", len(noReferenceSchema.Documents()), noReferenceReader.closeCount)
	}
}

func assertParseTestReferencedNilResolver(t *testing.T) {
	t.Helper()
	referenceReader := newParseTestReader(`<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"><xs:include schemaLocation="child"/></xs:schema>`)
	referenceRoot, err := goxsd9.NewResolvedSource(context.Background(), "reference-root", referenceReader)
	if err != nil {
		t.Fatalf("NewResolvedSource(reference): %v", err)
	}
	referenceSchema, err := goxsd9.ParseSchema(referenceRoot, nil)
	if err == nil {
		t.Fatal("ParseSchema with nil resolver accepted a reference")
	}
	diagnostic := parseTestDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureResolution || diagnostic.Code() != goxsd9.SourceResolveCode {
		t.Fatalf("nil-resolver diagnostic = %s, want source resolution diagnostic", diagnostic)
	}
	if len(referenceSchema.Documents()) != 0 || referenceReader.closeCount != 1 {
		t.Fatalf("nil-resolver error result = %d documents, close count %d", len(referenceSchema.Documents()), referenceReader.closeCount)
	}
}

func parseTestDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T does not contain a Diagnostic: %v", err, err)
	}
	return diagnostic
}

func parseTestQName(t *testing.T, namespace, local string) goxsd9.QName {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	return name
}

var _ io.ReadCloser = (*parseTestReader)(nil)
