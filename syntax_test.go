package goxsd9

import (
	"context"
	"errors"
	"io"
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
		reader := &trackingSource{data: []byte(input)}
		document, err := decodeResolvedSyntax(newTestSource(t, reader))
		if err != nil {
			_ = err.Error()
		}
		if document != nil && err != nil {
			t.Fatalf("decoder returned a document with an error: %v", err)
		}
		if !reader.closed {
			t.Fatal("decoder did not close fuzz source")
		}
	})
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
