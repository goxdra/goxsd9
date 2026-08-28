package specs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchVerifiesStatusAndChecksum(t *testing.T) {
	body := []byte("pinned response")
	cases := []fetchVerificationCase{
		{
			name:     "success",
			status:   http.StatusOK,
			digest:   testDigest(body),
			wantData: body,
		},
		{
			name:     "status",
			status:   http.StatusNotFound,
			digest:   testDigest(body),
			wantCode: "specs.network.status",
		},
		{
			name:     "checksum",
			status:   http.StatusOK,
			digest:   strings.Repeat("0", 64),
			wantCode: "specs.provenance.digest",
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			assertFetchVerification(t, test, body)
		})
	}
}

type fetchVerificationCase struct {
	name     string
	status   int
	digest   string
	wantCode string
	wantData []byte
}

func assertFetchVerification(t *testing.T, test fetchVerificationCase, body []byte) {
	t.Helper()
	entry := testEntry("xml", test.digest)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(test.status, body), nil
	})}
	data, err := Fetch(context.Background(), client, entry)
	if test.wantCode == "" {
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !bytes.Equal(data, test.wantData) {
			t.Fatalf("Fetch() data = %q, want %q", data, test.wantData)
		}
		return
	}
	if err == nil {
		t.Fatal("Fetch() error = nil")
	}
	assertErrorCode(t, err, test.wantCode)
}

func TestFetchPreservesReadAndCloseCauses(t *testing.T) {
	readCause := errors.New("read cause")
	closeCause := errors.New("close cause")
	entry := testEntry("xml", strings.Repeat("0", 64))
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       errorBody{readErr: readCause, closeErr: closeCause},
		}, nil
	})}
	_, err := Fetch(context.Background(), client, entry)
	if err == nil {
		t.Fatal("Fetch() error = nil")
	}
	assertErrorCode(t, err, "specs.network.read")
	if !errors.Is(err, readCause) || !errors.Is(err, closeCause) {
		t.Fatalf("Fetch() error = %v, want both read and close causes", err)
	}
}

func TestRepresentationConversionRequiresPinnedWrapper(t *testing.T) {
	content := []byte("<xs:schema>\n</xs:schema>\n")
	raw := append([]byte(cdataPrefix), content...)
	raw = append(raw, []byte(cdataSuffix+"\n")...)
	entry := testEntry("html-cdata-pre", testDigest(raw))
	converted, err := convert(entry, raw)
	if err != nil {
		t.Fatalf("convert() error = %v", err)
	}
	if !bytes.Equal(converted, content) {
		t.Fatalf("convert() = %q, want %q", converted, content)
	}

	for _, malformed := range [][]byte{
		append([]byte("<pre>"), []byte("body"+cdataSuffix+"\n")...),
		append([]byte(cdataPrefix), []byte("body"+cdataSuffix+"\n\n")...),
	} {
		entry.SHA256 = testDigest(malformed)
		_, generateErr := Generate(context.Background(), responseClient(malformed), entry)
		if generateErr == nil {
			t.Fatal("Generate() error = nil for malformed html-cdata-pre response")
		}
		assertErrorCode(t, generateErr, "specs.conversion.representation")
	}
	unsupported := testEntry("yaml", testDigest([]byte("body")))
	_, err = convert(unsupported, []byte("body"))
	if err == nil {
		t.Fatal("convert() error = nil for unsupported representation")
	}
	assertErrorCode(t, err, "specs.conversion.representation")
}

func TestGenerateValidatesBootstrapXMLAndPreservesConvertedBytes(t *testing.T) {
	content := []byte("<?xml version=\"1.0\"?>\n<!-- before -->\n<!DOCTYPE root SYSTEM \"root.dtd\">\n<root><![CDATA[ \t]]><?inside?><child/></root><!-- after -->\n")
	tests := []struct {
		name           string
		representation string
		raw            []byte
		want           []byte
	}{
		{
			name:           "xml",
			representation: "xml",
			raw:            content,
			want:           content,
		},
		{
			name:           "html-cdata-pre",
			representation: "html-cdata-pre",
			raw:            bootstrapXMLWrappedContent(content),
			want:           content,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			entry := testEntry(test.representation, testDigest(test.raw))
			entry.Kind = KindBootstrapArtifact
			document, err := Generate(context.Background(), responseClient(test.raw), entry)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if !bytes.Equal(document.Data, test.want) {
				t.Fatalf("Generate() data = %q, want %q", document.Data, test.want)
			}
		})
	}
}

func TestGenerateAcceptsBootstrapXMLDeclarationAndDoctypeForms(t *testing.T) {
	contents := []string{
		`<?xml version="1.0"?><root/>`,
		`<?xml version = '1.0' encoding = "UTF-8" standalone = 'yes'?>
<root/>`,
		`<!DOCTYPE root><root/>`,
		`<!DOCTYPE root SYSTEM "root.dtd"><root/>`,
		`<!DOCTYPE root PUBLIC "-//Example//DTD Root 1.0//EN" "root.dtd"><root/>`,
		`<!DOCTYPE root [<!-- DTD comment --><?dtd instruction?><!ELEMENT root EMPTY>]><root/>`,
	}
	for _, content := range contents {
		t.Run(fmt.Sprintf("%q", content), func(t *testing.T) {
			entry := testEntry("xml", testDigest([]byte(content)))
			entry.Kind = KindBootstrapArtifact
			document, err := Generate(context.Background(), responseClient([]byte(content)), entry)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if !bytes.Equal(document.Data, []byte(content)) {
				t.Fatalf("Generate() data = %q, want %q", document.Data, content)
			}
		})
	}
}

func TestGenerateRejectsMalformedBootstrapXML(t *testing.T) {
	tests := []bootstrapXMLInvalidCase{
		{name: "unclosed element", representation: "xml", content: "<root>"},
		{name: "trailing text", representation: "xml", content: "<root/>text"},
		{name: "second root", representation: "xml", content: "<one/><two/>"},
		{name: "late declaration", representation: "xml", content: "<root/><?xml version=\"1.0\"?>"},
		{name: "doctype after root", representation: "xml", content: "<root/><!DOCTYPE root>"},
		{name: "doctype before declaration", representation: "xml", content: "<!DOCTYPE root><?xml version=\"1.0\"?><root/>"},
		{name: "character data before root", representation: "xml", content: "text<root/>"},
		{name: "empty CDATA before root", representation: "xml", content: "<![CDATA[]]><root/>"},
		{name: "whitespace CDATA before root", representation: "xml", content: "<![CDATA[ \t\r\n]]><root/>"},
		{name: "whitespace CDATA after root", representation: "xml", content: "<root/><![CDATA[ \t\r\n]]>"},
		{name: "invalid directive", representation: "xml", content: "<!ENTITY root><root/>"},
		{name: "missing root", representation: "xml", content: "<!-- no root -->"},
		{name: "html-cdata-pre trailing text", representation: "html-cdata-pre", content: "<root/>text"},
		{name: "empty declaration", representation: "xml", content: "<?xml?><root/>"},
		{name: "empty declaration before newline", representation: "xml", content: "<?xml?>\n<root/>"},
		{name: "declaration without version", representation: "xml", content: "<?xml encoding=\"UTF-8\"?><root/>"},
		{name: "declaration with unknown field", representation: "xml", content: "<?xml version=\"1.0\" extra=\"value\"?><root/>"},
		{name: "empty doctype", representation: "xml", content: "<!DOCTYPE >\n<root/>"},
		{name: "doctype without external literal", representation: "xml", content: "<!DOCTYPE root SYSTEM><root/>"},
		{name: "duplicate attribute", representation: "xml", content: `<root id="one" id="two"/>`},
		{name: "duplicate expanded attribute", representation: "xml", content: `<root xmlns:a="urn:a" a:id="one" a:id="two"/>`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			assertRejectedBootstrapXML(t, test)
		})
	}
}

type bootstrapXMLInvalidCase struct {
	name           string
	representation string
	content        string
}

func assertRejectedBootstrapXML(t *testing.T, test bootstrapXMLInvalidCase) {
	t.Helper()
	raw := bootstrapXMLRaw(test.representation, []byte(test.content))
	entry := testEntry(test.representation, testDigest(raw))
	entry.Kind = KindBootstrapArtifact
	document, err := Generate(context.Background(), responseClient(raw), entry)
	if err == nil {
		t.Fatal("Generate() error = nil")
	}
	if document.Data != nil || document.Index != nil || document.Entry.ID != "" {
		t.Fatalf("Generate() document = %#v, want zero document", document)
	}
	assertErrorCode(t, err, bootstrapXMLDocumentCode)
	var corpusErr *Error
	if !errors.As(err, &corpusErr) {
		t.Fatalf("Generate() error = %v, want *Error", err)
	}
	if corpusErr.ID != entry.ID || corpusErr.URL != entry.URL {
		t.Fatalf("Generate() corpus location = %q / %q, want %q / %q", corpusErr.ID, corpusErr.URL, entry.ID, entry.URL)
	}
	var syntaxErr *xml.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("Generate() error = %v, want encoding/xml cause", err)
	}
	if !errors.Is(err, syntaxErr) {
		t.Fatalf("Generate() error = %v, does not unwrap encoding/xml cause", err)
	}
}

func bootstrapXMLRaw(representation string, content []byte) []byte {
	if representation != "html-cdata-pre" {
		return append([]byte(nil), content...)
	}
	return bootstrapXMLWrappedContent(content)
}

func bootstrapXMLWrappedContent(content []byte) []byte {
	raw := append([]byte(cdataPrefix), content...)
	return append(raw, []byte(cdataSuffix+"\n")...)
}

func TestXHTMLRenderingPreservesNavigationAndIndexesFragments(t *testing.T) {
	fixture := []byte(`<html>
  <head><title>ignored</title><style>.ignored { color: red }</style></head>
  <body>
    <h1 id="intro">Introduction</h1>
    <div>1 <a href="#intro">Introduction link</a></div>
    <p id="paragraph">Read <a href="guide/next.html#details">the <code>example</code></a> and <a name="repeat"></a>the repeated target.</p>
    <h2><a name="repeat" id="details"></a>Details</h2>
    <pre id="sample"><![CDATA[<xs:element name="item"/>
  value
]]></pre>
    <h3>No explicit target</h3>
  </body>
</html>`)
	first, err := renderHTML("https://www.w3.org/TR/2020/demo/", fixture)
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}
	second, err := renderHTML("https://www.w3.org/TR/2020/demo/", fixture)
	if err != nil {
		t.Fatalf("renderHTML() second error = %v", err)
	}
	if !bytes.Equal(first.data, second.data) {
		t.Fatal("renderHTML() output changed between identical runs")
	}
	if !equalIndex(first.entries, second.entries) {
		t.Fatal("renderHTML() index changed between identical runs")
	}

	output := string(first.data)
	contains := []string{
		`<a id="intro"></a>`,
		"# Introduction",
		"1 [Introduction link](#intro)",
		`<a id="paragraph"></a>`,
		`[the ` + "`example`" + `](https://www.w3.org/TR/2020/demo/guide/next.html#details)`,
		`<a name="repeat"></a>the repeated target.`,
		`<a name="repeat" id="details"></a>`,
		"```\n<xs:element name=\"item\"/>\n  value\n```",
		"### No explicit target",
	}
	for _, want := range contains {
		if !strings.Contains(output, want) {
			t.Fatalf("rendered output does not contain %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "ignored") || strings.Contains(output, "color: red") {
		t.Fatalf("rendered output retained head content:\n%s", output)
	}

	wantIndex := []IndexEntry{
		{Anchor: "intro", Level: 1, Occurrence: 1, Title: "Introduction"},
		{Anchor: "paragraph", Level: 1, Occurrence: 1, Title: "Introduction"},
		{Anchor: "repeat", Level: 1, Occurrence: 1, Title: "Introduction"},
		{Anchor: "repeat", Level: 2, Occurrence: 2, Title: "Details"},
		{Anchor: "details", Level: 2, Occurrence: 1, Title: "Details"},
		{Anchor: "sample", Level: 2, Occurrence: 1, Title: "Details"},
	}
	if !equalIndex(first.entries, wantIndex) {
		t.Fatalf("rendered index = %#v, want %#v", first.entries, wantIndex)
	}
}

func TestLegacyW3CHTMLRenderingRepairsVoidElementsAndEntities(t *testing.T) {
	fixture := []byte(`<!doctype html public '-//W3C//DTD HTML 4.0 Transitional//EN' 'http://www.w3.org/TR/REC-html40-971218/loose.dtd'><HTML><HEAD><link rel='STYLESHEET' type='text/css' href='xml.css'><meta http-equiv='Content-Type' content='text/html; charset=ISO-8859-1'><TITLE>ignored</TITLE></HEAD><BODY><P>Gr&uuml;&szlig; &Eacute; &auml; &ccedil; &eacute; &euml; &uuml; &amp;<!-- comment with > --><BR>Next</P></BODY></HTML>`)
	rendered, err := renderHTML("https://www.w3.org/TR/1999/REC-xml-names-19990114/", fixture)
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}
	output := string(rendered.data)
	if !strings.Contains(output, "Grüß É ä ç é ë ü &") {
		t.Fatalf("rendered output lost legacy HTML entities: %s", output)
	}
	if !strings.Contains(output, "Next") {
		t.Fatalf("rendered output lost content after a void element: %s", output)
	}
}

func TestLegacyW3CHTMLRenderingRepairsLatin1AndMismatchedClosings(t *testing.T) {
	fixture := []byte("<HTML><HEAD><meta http-equiv='Content-Type' content='text/html; charset=ISO-8859-1'></HEAD><BODY><DL><DT>Editors</DT><DD>caf\xe9</DL><P>After the list</P></BODY></HTML>")
	rendered, err := renderHTML("https://www.w3.org/TR/2001/REC-xmlbase-20010627/", fixture)
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}
	output := string(rendered.data)
	if !strings.Contains(output, "café") {
		t.Fatalf("rendered output lost ISO-8859-1 text: %s", output)
	}
	if !strings.Contains(output, "After the list") {
		t.Fatalf("rendered output lost content after mismatched closing tags: %s", output)
	}
}

func TestWritePublishesDeterministicArtifacts(t *testing.T) {
	outputDir := t.TempDir()
	document := Document{
		Entry: Entry{ID: "demo", Representation: "html"},
		Data:  []byte("# Demo\n"),
		Index: []IndexEntry{{Anchor: "demo", Level: 1, Occurrence: 1, Title: "Demo"}},
	}
	paths, err := Write(outputDir, document)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("Write() paths = %#v, want two artifacts", paths)
	}
	dataPath := filepath.Join(outputDir, "demo.md")
	indexPath := filepath.Join(outputDir, "demo.index")
	// #nosec G304 -- dataPath is constructed below t.TempDir().
	data, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("ReadFile(data) error = %v", err)
	}
	if string(data) != "# Demo\n" {
		t.Fatalf("data artifact = %q", data)
	}
	writeErr := os.WriteFile(dataPath, []byte("old\n"), 0o600)
	if writeErr != nil {
		t.Fatalf("WriteFile(old) error = %v", writeErr)
	}
	document.Data = []byte("# Replaced\n")
	_, replaceErr := Write(outputDir, document)
	if replaceErr != nil {
		t.Fatalf("Write() replacement error = %v", replaceErr)
	}
	// #nosec G304 -- dataPath is constructed below t.TempDir().
	replaced, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("ReadFile(replaced) error = %v", err)
	}
	if string(replaced) != "# Replaced\n" {
		t.Fatalf("replaced artifact = %q", replaced)
	}
	_, statErr := os.Stat(indexPath)
	if statErr != nil {
		t.Fatalf("index artifact missing: %v", statErr)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".goxsd9-specs-") {
			t.Fatalf("temporary artifact left behind: %s", entry.Name())
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type errorBody struct {
	readErr  error
	closeErr error
}

func (body errorBody) Read([]byte) (int, error) {
	return 0, body.readErr
}

func (body errorBody) Close() error {
	return body.closeErr
}

func testEntry(representation, digest string) Entry {
	return Entry{
		ID:             "demo",
		Representation: representation,
		SHA256:         digest,
		URL:            "https://www.w3.org/TR/2020/demo/",
	}
}

func testResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d test status", status),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func responseClient(body []byte) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(http.StatusOK, body), nil
	})}
}

func testDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func equalIndex(left, right []IndexEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
