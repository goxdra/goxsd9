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

//nolint:gocognit,funlen // Keep the end-to-end immutable and deterministic contract together.
func TestParseSchemaModelsGlobalElementScalarTypesAcrossMixedDocuments(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:wrong" xmlns:o="urn:other" targetNamespace="urn:root" version="1.0">
  <xs:import namespace="urn:other" schemaLocation="other-location"/>
  <xs:element name="integerValue" type="  xs:integer  "/>
  <xs:element name="decimalValue" type="xs:decimal"/>
  <xs:element xmlns:t="urn:root" name="namedValue" type=" t:Named "/>
  <xs:element name="crossValue" type=" o:OtherAmount "/>
  <xs:simpleType name="Named"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	childContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:other" version="1.1">
  <xs:simpleType name="OtherAmount"><xs:restriction base="xs:integer"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType>
</xs:schema>`
	rootReader := newParseTestReader(rootContents)
	root, err := goxsd9.NewResolvedSource(context.Background(), "typed-root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &typedElementParseResolver{contents: childContents}
	schema, err := goxsd9.ParseSchema(root, resolver)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	if got, want := len(resolver.calls), 1; got != want {
		t.Fatalf("resolver call count = %d, want %d; type references must not resolve sources", got, want)
	}
	if got, want := resolver.calls[0], (parseTestCall{namespaceURN: "urn:other", location: "other-location"}); got != want {
		t.Fatalf("resolver call = %#v, want %#v", got, want)
	}
	if got, want := rootReader.closeCount, 1; got != want {
		t.Fatalf("root close count = %d, want %d", got, want)
	}
	components := schema.Components()
	if got, want := len(components), 6; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	wantNames := []goxsd9.QName{
		parseTestQName(t, "urn:root", "integerValue"),
		parseTestQName(t, "urn:root", "decimalValue"),
		parseTestQName(t, "urn:root", "namedValue"),
		parseTestQName(t, "urn:root", "crossValue"),
		parseTestQName(t, "urn:root", "Named"),
		parseTestQName(t, "urn:other", "OtherAmount"),
	}
	for index, want := range wantNames {
		if got := components[index].Name(); got != want {
			t.Errorf("component %d name = %q, want %q", index, got, want)
		}
	}
	assertParseElementView(t, components[0], parseTestQName(t, parseTestXSDNamespace, "integer"), goxsd9.ComponentID{}, false)
	assertParseElementView(t, components[1], parseTestQName(t, parseTestXSDNamespace, "decimal"), goxsd9.ComponentID{}, false)
	assertParseElementView(t, components[2], parseTestQName(t, "urn:root", "Named"), components[4].ID(), true)
	assertParseElementView(t, components[3], parseTestQName(t, "urn:other", "OtherAmount"), components[5].ID(), true)
	if _, ok := components[4].Element(); ok {
		t.Fatal("simple type component has an element view")
	}
	namedType, ok := components[4].SimpleType()
	if !ok {
		t.Fatal("root named simple type view is missing")
	}
	if got, want := namedType.DigitFacets().Version(), goxsd9.XSDVersion10; got != want {
		t.Fatalf("root named type version = %q, want %q", got, want)
	}
	otherType, ok := components[5].SimpleTypeDefinition()
	if !ok {
		t.Fatal("cross-document simple type view is missing")
	}
	if got, want := otherType.DigitFacets().Version(), goxsd9.XSDVersion11; got != want {
		t.Fatalf("cross-document type version = %q, want %q", got, want)
	}
	walked := make([]goxsd9.ComponentID, 0, len(components))
	if err := schema.Walk(func(component goxsd9.Component) error {
		walked = append(walked, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	wantWalk := make([]goxsd9.ComponentID, 0, len(components))
	for _, component := range components {
		wantWalk = append(wantWalk, component.ID())
	}
	if !reflect.DeepEqual(walked, wantWalk) {
		t.Fatalf("Walk IDs = %#v, want %#v", walked, wantWalk)
	}
	namedName := parseTestQName(t, "urn:root", "namedValue")
	allFound := schema.Find(namedName)
	if len(allFound) != 1 || allFound[0].ID() != components[2].ID() {
		t.Fatalf("Find(namedValue) = %#v, want component 2", allFound)
	}
	found := schema.FindKind(goxsd9.ComponentKindElementDeclaration, namedName)
	if len(found) != 1 || found[0].ID() != components[2].ID() {
		t.Fatalf("FindKind(namedValue) = %#v, want component 2", found)
	}
	elementView, ok := components[2].Element()
	if !ok {
		t.Fatal("named element view is missing")
	}
	alias, ok := components[2].ElementDeclaration()
	if !ok || alias.DeclaredType() != elementView.DeclaredType() {
		t.Fatal("ElementDeclaration compatibility alias does not preserve declared type")
	}
	assertParseElementCopies(t, schema, namedName, components[2].ID())
}

func assertParseElementView(t *testing.T, component goxsd9.Component, wantType goxsd9.QName, wantTypeID goxsd9.ComponentID, wantHasTypeID bool) {
	t.Helper()
	view, ok := component.Element()
	if !ok {
		t.Fatalf("component %s has no element view", component.ID().Source())
	}
	if view.Component().ID() != component.ID() || view.ID() != component.ID() || view.Name() != component.Name() || view.Loc() != component.Loc() {
		t.Fatalf("element view does not preserve generic component facts")
	}
	if got := view.DeclaredType(); got != wantType {
		t.Fatalf("declared type = %q, want %q", got, wantType)
	}
	gotTypeID, gotHasTypeID := view.TypeID()
	if gotTypeID != wantTypeID || gotHasTypeID != wantHasTypeID {
		t.Fatalf("type ID = (%v, %t), want (%v, %t)", gotTypeID, gotHasTypeID, wantTypeID, wantHasTypeID)
	}
}

func assertParseElementCopies(t *testing.T, schema goxsd9.Schema, name goxsd9.QName, wantID goxsd9.ComponentID) {
	t.Helper()
	components := schema.Components()
	components[2] = goxsd9.Component{}
	if got, ok := schema.Lookup(wantID); !ok || got.Name() != name {
		t.Fatal("mutating Components changed the completed element")
	}
	found := schema.Find(name)
	found[0] = goxsd9.Component{}
	if got, ok := schema.Lookup(wantID); !ok || got.Name() != name {
		t.Fatal("mutating Find changed the completed element")
	}
	documents := schema.Documents()
	documentComponents := documents[0].Components()
	documentComponents[2] = goxsd9.Component{}
	if got, ok := schema.Lookup(wantID); !ok || got.Name() != name {
		t.Fatal("mutating document Components changed the completed element")
	}
}

//nolint:gocognit // Keep the end-to-end public particle contract together.
func TestParseSchemaExposesConcreteChoiceParticleAlternatives(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element name="integer" type="xs:integer"/>
      <xs:element name="decimal" type="xs:decimal"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
	root, err := goxsd9.NewResolvedSource(context.Background(), "choice-root", newParseTestReader(rootContents))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	schema, err := goxsd9.ParseSchema(root, nil)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 1; got != want {
		t.Fatalf("global component count = %d, want %d", got, want)
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("complex type definition view is missing")
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatalf("particle type = %T, want ChoiceParticle", definition.Particle())
	}
	if choice.MinOccurs() != 1 || choice.MaxOccurs() != 1 {
		t.Fatalf("choice occurrence bounds = %d/%d, want 1/1", choice.MinOccurs(), choice.MaxOccurs())
	}
	alternatives := choice.Alternatives()
	if got, want := len(alternatives), 2; got != want {
		t.Fatalf("alternative count = %d, want %d", got, want)
	}
	alternatives[0] = nil
	if len(choice.Alternatives()) != 2 {
		t.Fatal("mutating returned alternatives changed the completed schema")
	}
	for index, wantName := range []string{"integer", "decimal"} {
		element, ok := choice.Alternatives()[index].(goxsd9.ElementParticle)
		if !ok {
			t.Fatalf("alternative %d type = %T, want ElementParticle", index, choice.Alternatives()[index])
		}
		if got := element.Name().Namespace(); got != "" {
			t.Fatalf("alternative %d name namespace = %q, want empty", index, got)
		}
		if got, want := element.Name().Local(), wantName; got != want {
			t.Fatalf("alternative %d name = %q, want %q", index, got, want)
		}
		if typeID, hasTypeID := element.TypeID(); hasTypeID || !typeID.IsZero() {
			t.Fatalf("alternative %d built-in type has a synthetic ID", index)
		}
	}
}

type typedElementParseResolver struct {
	calls    []parseTestCall
	contents string
}

func (resolver *typedElementParseResolver) Resolve(ctx context.Context, namespaceURN, schemaLocation string) (goxsd9.ResolvedSource, error) {
	resolver.calls = append(resolver.calls, parseTestCall{namespaceURN: namespaceURN, location: schemaLocation})
	if schemaLocation != "other-location" {
		return goxsd9.ResolvedSource{}, fmt.Errorf("unexpected schema location %q", schemaLocation)
	}
	childContext := context.WithValue(ctx, parseTestContextKey{}, "typed-other")
	return goxsd9.NewResolvedSource(childContext, "typed-other", newParseTestReader(resolver.contents))
}

func assertParseTestDocuments(t *testing.T, schema goxsd9.Schema) {
	t.Helper()
	wantSources := []goxsd9.SourceID{"opaque-root", "opaque-a", "opaque-b"}
	wantNamespaces := []string{"urn:root", "urn:root", "urn:b"}
	wantRootLocs := []goxsd9.Loc{
		parseTestLoc(t, "opaque-root", 1, 1),
		parseTestLoc(t, "opaque-a", 1, 1),
		parseTestLoc(t, "opaque-b", 1, 1),
	}
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
		if got := documents[index].RootLoc(); got != wantRootLocs[index] {
			t.Errorf("document %d root location = %s, want %s", index, got, wantRootLocs[index])
		}
	}
}

func TestParseSchemaExposesUnicodeRootLocation(t *testing.T) {
	rootContents := "\n  <!-- λβ --> <xs:schema xmlns:xs=\"" + parseTestXSDNamespace + "\"/>"
	rootReader := newParseTestReader(rootContents)
	root, err := goxsd9.NewResolvedSource(context.Background(), "unicode-root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}

	schema, err := goxsd9.ParseSchema(root, nil)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	documents := schema.Documents()
	if got, want := len(documents), 1; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	wantLoc := parseTestLoc(t, "unicode-root", 2, 15)
	if got := documents[0].RootLoc(); got != wantLoc {
		t.Fatalf("root location = %s, want %s", got, wantLoc)
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
	if got := schema.Documents()[0].RootLoc(); got != parseTestLoc(t, "opaque-root", 1, 1) {
		t.Fatalf("mutating Documents changed root location to %s", got)
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

func parseTestLoc(t *testing.T, source goxsd9.SourceID, line, column int) goxsd9.Loc {
	t.Helper()
	loc, err := goxsd9.NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

var _ io.ReadCloser = (*parseTestReader)(nil)
