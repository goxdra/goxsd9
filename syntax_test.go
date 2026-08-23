package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const testXSDNamespace = "http://www.w3.org/2001/XMLSchema"

func TestDecodeSyntaxBuildsOrderedLocatedTree(t *testing.T) {
	input := "<?xml version=\"1.0\"?>\n" +
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\">\n" +
		"  <xs:element name=\"root\">\n" +
		"    <xs:complexType><xs:sequence><xs:element name=\"child\"/></xs:sequence></xs:complexType>\n" +
		"  </xs:element>\n" +
		"</xs:schema>"
	reader := &trackingSource{data: []byte(input)}
	document := decodeTestSource(t, reader)

	if got, want := document.source, SourceID("schema.xsd"); got != want {
		t.Fatalf("document source = %q, want %q", got, want)
	}
	if got, want := document.root.name, (syntaxName{namespace: testXSDNamespace, local: "schema"}); got != want {
		t.Fatalf("root name = %#v, want %#v", got, want)
	}
	assertLoc(t, document.root.loc, 2, 1)
	if got, want := len(document.root.children), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	element, ok := document.root.children[1].(*syntaxElement)
	if !ok {
		t.Fatalf("root child 1 = %T, want *syntaxElement", document.root.children[1])
	}
	if got, want := element.name.local, "element"; got != want {
		t.Fatalf("declaration name = %q, want %q", got, want)
	}
	assertLoc(t, element.loc, 3, 3)
	if got, want := len(element.children), 3; got != want {
		t.Fatalf("element child count = %d, want %d", got, want)
	}
	if namespace, ok := element.scope.lookup("xs"); !ok || namespace != testXSDNamespace {
		t.Fatalf("element xs binding = %q, %t; want %q, true", namespace, ok, testXSDNamespace)
	}
	if !reader.closed {
		t.Fatal("decoder did not close a successful source")
	}
	if got, want := reader.offset, len(reader.data); got != want {
		t.Fatalf("successful decode consumed %d bytes, want %d", got, want)
	}
}

func TestDecodeSyntaxCountsUnicodeCodePointColumns(t *testing.T) {
	input := "<xs:schema xmlns:xs=\"" + testXSDNamespace + "\">\n" +
		"😀<xs:assertion/>\n</xs:schema>"
	reader := &trackingSource{data: []byte(input)}
	source := newTestSource(t, reader)
	_, err := decodeResolvedSyntax(source)
	diagnostic := requireDiagnostic(t, err)

	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Feature(), FeatureID("xsd.assertion"); got != want {
		t.Fatalf("Feature() = %q, want %q", got, want)
	}
	assertLoc(t, diagnostic.Loc(), 2, 2)
	if got, want := diagnostic.SpecRef(), "xsd11-structures#cAssertions"; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
	if !reader.closed || reader.offset != len(reader.data) {
		t.Fatalf("unsupported decode did not drain and close source: closed=%t offset=%d want=%d", reader.closed, reader.offset, len(reader.data))
	}
}

func TestDecodeSyntaxNormalizesXMLLineEndings(t *testing.T) {
	for _, separator := range []string{"\r", "\r\n"} {
		t.Run(strings.ReplaceAll(separator, "\r", "CR"), func(t *testing.T) {
			input := "<xs:schema xmlns:xs=\"" + testXSDNamespace + "\">" + separator +
				"😀<xs:assertion/></xs:schema>"
			reader := &trackingSource{data: []byte(input)}
			_, err := decodeResolvedSyntax(newTestSource(t, reader))
			diagnostic := requireDiagnostic(t, err)
			assertLoc(t, diagnostic.Loc(), 2, 2)
		})
	}
}

func TestDecodeSyntaxAcceptsUTF8BOMBeforeWhitespace(t *testing.T) {
	input := "\ufeff \r\n<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"/>"
	reader := &trackingSource{data: []byte(input)}
	document := decodeTestSource(t, reader)
	assertLoc(t, document.root.loc, 2, 1)
}

func TestDecodeSyntaxClassifiesMalformedXMLAndWrongRoots(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  string
		line  int
		col   int
	}{
		{
			name:  "mismatched end tag",
			input: "<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><xs:element></xs:schema>",
			code:  InvalidXMLSyntaxCode,
			line:  1,
			col:   68,
		},
		{
			name:  "wrong root",
			input: "<root/>",
			code:  InvalidSchemaRootCode,
			line:  1,
			col:   1,
		},
		{
			name:  "no root",
			input: " \n\t",
			code:  InvalidSchemaRootCode,
			line:  2,
			col:   2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &trackingSource{data: []byte(test.input)}
			_, err := decodeResolvedSyntax(newTestSource(t, reader))
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), test.code; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			assertLoc(t, diagnostic.Loc(), test.line, test.col)
			if !reader.closed || reader.offset != len(reader.data) {
				t.Fatalf("invalid decode did not drain and close source: closed=%t offset=%d want=%d", reader.closed, reader.offset, len(reader.data))
			}
		})
	}
}

func TestDecodeSyntaxRejectsUnboundNamespacesAndDuplicateAttributes(t *testing.T) {
	tests := []string{
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><u:element/></xs:schema>",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><xs:element a=\"1\" a=\"2\"/></xs:schema>",
	}
	for _, input := range tests {
		reader := &trackingSource{data: []byte(input)}
		_, err := decodeResolvedSyntax(newTestSource(t, reader))
		diagnostic := requireDiagnostic(t, err)
		if got, want := diagnostic.Class(), FailureInvalid; got != want {
			t.Fatalf("Class() = %q, want %q", got, want)
		}
		if got, want := diagnostic.Code(), InvalidXMLSyntaxCode; got != want {
			t.Fatalf("Code() = %q, want %q", got, want)
		}
	}
}

func TestDecodeSyntaxRejectsXMLNamespaceAliases(t *testing.T) {
	tests := []string{
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\" xmlns:xmlish=\"" + xmlNamespaceURI + "\"/>",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\" xmlns=\"" + xmlNamespaceURI + "\"/>",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\" xmlns:xml=\"urn:not-xml\"/>",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := decodeResolvedSyntax(newTestSource(t, &trackingSource{data: []byte(input)}))
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), FailureInvalid)
			}
			if diagnostic.Code() != InvalidXMLSyntaxCode {
				t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), InvalidXMLSyntaxCode)
			}
		})
	}

	valid := "<xs:schema xmlns:xs=\"" + testXSDNamespace + "\" xmlns:xml=\"" + xmlNamespaceURI + "\"/>"
	if _, err := decodeResolvedSyntax(newTestSource(t, &trackingSource{data: []byte(valid)})); err != nil {
		t.Fatalf("decodeResolvedSyntax rejected the canonical xml binding: %v", err)
	}
}

func TestDecodeSyntaxReportsGenericUnsupportedSyntax(t *testing.T) {
	input := "<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><xs:alternative/></xs:schema>"
	reader := &trackingSource{data: []byte(input)}
	_, err := decodeResolvedSyntax(newTestSource(t, reader))
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Feature(), FeatureSchemaSyntax; got != want {
		t.Fatalf("Feature() = %q, want %q", got, want)
	}
	if got, want := diagnostic.SpecRef(), "xsd10-structures#schema-document"; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
}

func TestParseSchemaReportsXMLBaseAsUnsupportedResolution(t *testing.T) {
	input := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="child.xsd"/>`
	reader := &trackingSource{data: []byte(input)}
	root := newTestSource(t, reader)
	schema, err := ParseSchema(root, nil)
	if err == nil {
		t.Fatal("ParseSchema accepted XML Base")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported {
		t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), FailureUnsupported)
	}
	if diagnostic.Feature() != FeatureID("xsd.schema.xml-base") {
		t.Fatalf("diagnostic feature = %q, want xsd.schema.xml-base", diagnostic.Feature())
	}
	if diagnostic.SpecRef() != "xmlbase#matching" {
		t.Fatalf("diagnostic spec ref = %q, want xmlbase#matching", diagnostic.SpecRef())
	}
	if diagnostic.Loc().Source() != "schema.xsd" || diagnostic.Loc().Line() != 1 || diagnostic.Loc().Column() != 1 {
		t.Fatalf("diagnostic location = %s, want schema.xsd:1:1", diagnostic.Loc())
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatalf("schema after XML Base error = %#v, want zero schema", schema)
	}
	if !reader.closed {
		t.Fatal("XML Base source was not closed")
	}
}

func TestDecodeSyntaxPreservesCloseFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	reader := &trackingSource{data: []byte("<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"/>")}
	reader.closeErr = closeErr
	_, err := decodeResolvedSyntax(newTestSource(t, reader))
	if err == nil {
		t.Fatal("decode succeeded despite close failure")
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("decode error does not preserve close failure: %v", err)
	}
	if !reader.closed {
		t.Fatal("decoder did not attempt to close source")
	}
}

func TestDecodeSyntaxPreservesReadFailureAndCloses(t *testing.T) {
	readErr := errors.New("read failed")
	input := []byte("<xs:schema xmlns:xs=\"" + testXSDNamespace + "\">")
	reader := &trackingSource{data: input, failAt: 12, readErr: readErr}
	_, err := decodeResolvedSyntax(newTestSource(t, reader))
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureResolution; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), SourceReadCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if !errors.Is(err, readErr) {
		t.Fatalf("decode error does not preserve read failure: %v", err)
	}
	if !reader.closed || reader.offset != reader.failAt {
		t.Fatalf("read failure did not close at expected offset: closed=%t offset=%d want=%d", reader.closed, reader.offset, reader.failAt)
	}
}

func FuzzDecodeSyntax(f *testing.F) {
	for _, seed := range []string{
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"/>",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><xs:element name=\"root\"/></xs:schema>",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><xs:assertion/></xs:schema>",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\"><xs:element></xs:schema>",
		"not XML",
		"<xs:schema xmlns:xs=\"" + testXSDNamespace + "\">😀</xs:schema>",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		first := fuzzDecodeRun(t, input)
		second := fuzzDecodeRun(t, input)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("decoder result is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
		}
	})
}

type fuzzDecodeResult struct {
	document     *syntaxDocument
	diagnostics  []fuzzDiagnostic
	closed       bool
	consumedByte int
}

type fuzzDiagnostic struct {
	class   FailureClass
	code    string
	feature FeatureID
	loc     Loc
	message string
	specRef string
}

func fuzzDecodeRun(t *testing.T, input string) fuzzDecodeResult {
	t.Helper()
	reader := &trackingSource{data: []byte(input)}
	document, err := decodeResolvedSyntax(newTestSource(t, reader))
	if document != nil && err != nil {
		t.Fatalf("decoder returned a document with an error: %v", err)
	}
	if document == nil && err == nil {
		t.Fatal("decoder returned neither a document nor an error")
	}
	if !reader.closed {
		t.Fatal("decoder did not close fuzz source")
	}
	if reader.offset != len(reader.data) {
		t.Fatalf("decoder did not drain fuzz source: consumed %d of %d bytes", reader.offset, len(reader.data))
	}
	result := fuzzDecodeResult{
		document:     document,
		closed:       reader.closed,
		consumedByte: reader.offset,
	}
	if err == nil {
		return result
	}
	for _, diagnostic := range syntaxDiagnostics(err) {
		result.diagnostics = append(result.diagnostics, fuzzDiagnostic{
			class:   diagnostic.Class(),
			code:    diagnostic.Code(),
			feature: diagnostic.Feature(),
			loc:     diagnostic.Loc(),
			message: diagnostic.Error(),
			specRef: diagnostic.SpecRef(),
		})
	}
	if len(result.diagnostics) == 0 {
		t.Fatalf("decoder returned an error without diagnostics: %v", err)
	}
	return result
}

func decodeTestSource(t *testing.T, reader *trackingSource) *syntaxDocument {
	t.Helper()
	source := newTestSource(t, reader)
	document, err := decodeSyntax(source.stream(), syntaxDecodeConfig{sourceID: source.SourceID()})
	if err != nil {
		t.Fatalf("decodeSyntax: %v", err)
	}
	return document
}

func newTestSource(t *testing.T, reader io.ReadCloser) ResolvedSource {
	t.Helper()
	source, err := NewResolvedSource(context.Background(), "schema.xsd", reader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	return source
}

func requireDiagnostic(t *testing.T, err error) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostic")
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T does not contain Diagnostic: %v", err, err)
	}
	return diagnostic
}

func assertLoc(t *testing.T, loc Loc, line, column int) {
	t.Helper()
	if got, want := loc.Line(), line; got != want {
		t.Fatalf("line = %d, want %d (%s)", got, want, loc)
	}
	if got, want := loc.Column(), column; got != want {
		t.Fatalf("column = %d, want %d (%s)", got, want, loc)
	}
}

type trackingSource struct {
	data     []byte
	offset   int
	failAt   int
	readErr  error
	closeErr error
	closed   bool
}

func (source *trackingSource) Read(buffer []byte) (int, error) {
	if source.failAt > 0 && source.offset >= source.failAt {
		return 0, source.readErr
	}
	if source.offset >= len(source.data) {
		return 0, io.EOF
	}
	limit := len(source.data)
	if source.failAt > 0 && source.failAt < limit {
		limit = source.failAt
	}
	n := copy(buffer, source.data[source.offset:limit])
	source.offset += n
	return n, nil
}

func (source *trackingSource) Close() error {
	source.closed = true
	return source.closeErr
}
