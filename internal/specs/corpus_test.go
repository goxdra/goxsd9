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
	"strconv"
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

func TestXSD10DatatypesConversionMovesOnlyDeclaration(t *testing.T) {
	content := "<!DOCTYPE root SYSTEM \"root>schema.dtd\" [\n" +
		"<!-- DTD comment with > -->\n" +
		"<?dtd instruction >?>\n" +
		"<!ELEMENT root EMPTY>\n" +
		"]>\n\n" +
		"<?xml version='1.0' encoding=\"UTF-8\"?>\n" +
		"<root><![CDATA[<?xml version='1.0'?>]]></root>\n"
	raw := bootstrapXMLWrappedContent([]byte(content))
	entry := xsd10DatatypesTestEntry(testDigest(raw))
	document, err := Generate(context.Background(), responseClient(raw), entry)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := []byte("<?xml version='1.0' encoding=\"UTF-8\"?><!DOCTYPE root SYSTEM \"root>schema.dtd\" [\n" +
		"<!-- DTD comment with > -->\n" +
		"<?dtd instruction >?>\n" +
		"<!ELEMENT root EMPTY>\n" +
		"]>\n\n\n" +
		"<root><![CDATA[<?xml version='1.0'?>]]></root>\n")
	if !bytes.Equal(document.Data, want) {
		t.Fatalf("Generate() data = %q, want %q", document.Data, want)
	}
}

func TestXSD10DatatypesConversionRejectsEnvelopeDrift(t *testing.T) {
	const declaration = "<?xml version='1.0'?>"
	const doctype = "<!DOCTYPE root [<!ELEMENT root EMPTY>]>"
	validContent := doctype + "\n\n" + declaration + "\n<root/>\n"
	validRaw := bootstrapXMLWrappedContent([]byte(validContent))
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "missing wrapper prefix", raw: append([]byte("<pre>"), validRaw[len(cdataPrefix):]...)},
		{name: "missing wrapper suffix", raw: validRaw[:len(validRaw)-len(cdataSuffix)-1]},
		{name: "trailing wrapper content", raw: append(append([]byte(nil), validRaw...), []byte("tail")...)},
		{name: "missing DTD", raw: bootstrapXMLWrappedContent([]byte(declaration + "\n<root/>\n"))},
		{name: "declaration before DTD", raw: bootstrapXMLWrappedContent([]byte(declaration + "\n" + validContent))},
		{name: "missing declaration", raw: bootstrapXMLWrappedContent([]byte(doctype + "\n\n<root/>\n"))},
		{name: "malformed DTD", raw: bootstrapXMLWrappedContent([]byte("<!DOCTYPE root [<!ELEMENT root EMPTY>\n\n" + declaration + "\n<root/>\n"))},
		{name: "DTD separator drift", raw: bootstrapXMLWrappedContent([]byte(doctype + "\n" + declaration + "\n<root/>\n"))},
		{name: "malformed declaration", raw: bootstrapXMLWrappedContent([]byte(doctype + "\n\n<?xml version='1.0' extra='value'?>\n<root/>\n"))},
		{name: "repeated declaration", raw: bootstrapXMLWrappedContent([]byte(validContent + declaration))},
		{name: "repeated DTD", raw: bootstrapXMLWrappedContent([]byte(validContent + "<!DOCTYPE root>"))},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			entry := xsd10DatatypesTestEntry(testDigest(test.raw))
			document, err := Generate(context.Background(), responseClient(test.raw), entry)
			if err == nil {
				t.Fatal("Generate() error = nil")
			}
			if document.Data != nil || document.Index != nil || document.Entry.ID != "" {
				t.Fatalf("Generate() document = %#v, want zero document", document)
			}
			assertErrorCode(t, err, "specs.conversion.representation")
			var corpusErr *Error
			if !errors.As(err, &corpusErr) || corpusErr.ID != entry.ID || corpusErr.URL != entry.URL {
				t.Fatalf("Generate() error = %v, want entry location", err)
			}
		})
	}
}

func TestXSD10DatatypesDigestPrecedesConversion(t *testing.T) {
	raw := []byte("not the pinned envelope")
	entry := xsd10DatatypesTestEntry(testDigest([]byte("different raw response")))
	_, err := Generate(context.Background(), responseClient(raw), entry)
	if err == nil {
		t.Fatal("Generate() error = nil")
	}
	assertErrorCode(t, err, "specs.provenance.digest")
	if strings.Contains(err.Error(), "specs.conversion.representation") {
		t.Fatalf("Generate() converted before digest failure: %v", err)
	}
}

func TestXSD10DatatypesXMLFailurePreservesCause(t *testing.T) {
	raw := bootstrapXMLWrappedContent([]byte("<!DOCTYPE root [<!ELEMENT root EMPTY>]>\n\n" +
		"<?xml version='1.0'?>\n<root>\n"))
	entry := xsd10DatatypesTestEntry(testDigest(raw))
	document, err := Generate(context.Background(), responseClient(raw), entry)
	if err == nil {
		t.Fatal("Generate() error = nil")
	}
	if document.Data != nil || document.Entry.ID != "" {
		t.Fatalf("Generate() document = %#v, want zero document", document)
	}
	assertErrorCode(t, err, bootstrapXMLDocumentCode)
	var syntaxErr *xml.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("Generate() error = %v, want XML syntax cause", err)
	}
	var corpusErr *Error
	if !errors.As(err, &corpusErr) || corpusErr.ID != entry.ID || corpusErr.URL != entry.URL {
		t.Fatalf("Generate() error = %v, want entry location", err)
	}
}

func TestXSD10DatatypesConversionRequiresPinnedBootstrapEntry(t *testing.T) {
	content := "<!DOCTYPE root [<!ELEMENT root EMPTY>]>\n\n<?xml version='1.0'?>\n<root/>\n"
	raw := bootstrapXMLWrappedContent([]byte(content))
	tests := []struct {
		name   string
		mutate func(*Entry)
	}{
		{
			name: "wrong ID",
			mutate: func(entry *Entry) {
				entry.ID = "other-artifact"
			},
		},
		{
			name: "wrong kind",
			mutate: func(entry *Entry) {
				entry.Kind = KindSpecification
			},
		},
		{
			name: "entry artifact",
			mutate: func(entry *Entry) {
				entry.Entry = true
			},
		},
		{
			name: "wrong XSD version",
			mutate: func(entry *Entry) {
				entry.XSDVersions = []string{"1.1"}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			entry := xsd10DatatypesTestEntry(testDigest(raw))
			test.mutate(&entry)
			document, err := Generate(context.Background(), responseClient(raw), entry)
			if err == nil {
				t.Fatal("Generate() error = nil")
			}
			if document.Data != nil || document.Index != nil || document.Entry.ID != "" {
				t.Fatalf("Generate() document = %#v, want zero document", document)
			}
			assertErrorCode(t, err, "specs.conversion.representation")
			var corpusErr *Error
			if !errors.As(err, &corpusErr) || corpusErr.ID != entry.ID || corpusErr.URL != entry.URL {
				t.Fatalf("Generate() error = %v, want entry location", err)
			}
		})
	}
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

func TestGenerateAcceptsLeadingUTF8BOM(t *testing.T) {
	content := append([]byte(bootstrapXMLUTF8BOM), []byte(`<?xml version="1.0"?><root/>`)...)
	entry := testEntry("xml", testDigest(content))
	entry.Kind = KindBootstrapArtifact
	document, err := Generate(context.Background(), responseClient(content), entry)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !bytes.Equal(document.Data, content) {
		t.Fatalf("Generate() data = %q, want %q", document.Data, content)
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
		`<!DOCTYPE root [<?pi %literal;?>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (child|other)*><!ATTLIST root id CDATA #IMPLIED><!ENTITY name "value"><!NOTATION image SYSTEM "image">]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (child)>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (child)?>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (((child)))>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root ((child|other),child?)><!ELEMENT child EMPTY><!ELEMENT other EMPTY><!ATTLIST root id ID #IMPLIED mode (one|two) "one" kind NOTATION (image|text) #FIXED "image"><!ENTITY name "hello &amp; world"><!ENTITY external SYSTEM "root.ent"><!NOTATION image PUBLIC "-//Example//Image//EN" "image.bin"><!NOTATION text PUBLIC "-//Example//Text//EN">]><root/>`,
		`<!DOCTYPE root [<!ENTITY markup '<child/>'>]><root>&markup;</root>`,
		`<!DOCTYPE root [<!ENTITY markup '&#60;child/>'>]><root>&markup;</root>`,
		`<!DOCTYPE root [<!ENTITY markup '<p:child xmlns:p="urn:p" id="one"/>'>]><root>&markup;</root>`,
		`<!DOCTYPE root [<!ENTITY cdata '<![CDATA[<]]>'>]><root>&cdata;</root>`,
		`<!DOCTYPE root [<!ENTITY e 'ok'>]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY e '&#x41;'>]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY e '&amp;&gt;&apos;&quot;'>]><root>&e;</root>`,
		`<?a:b:c data?><root/>`,
		`<root>&lt;&#60;</root>`,
		`<root value="&lt;&#60;"/>`,
		`<!DOCTYPE root [<!ATTLIST root value CDATA '&lt;'><!ATTLIST root numeric CDATA '&#60;'>]><root/>`,
		`<!DOCTYPE root [<!ENTITY e '&#38;#60;'>]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY e '&#38;#60;'>]><root value="&e;"/>`,
		`<!DOCTYPE root [<!ENTITY e '&amp;#60;'>]><root>&e;</root>`,
		`<root>%text;</root>`,
		`<!DOCTYPE root [<!ENTITY e 'first'><!ENTITY e SYSTEM 'root.ent'>]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY external SYSTEM 'root.ent'><!ENTITY internal '&external;'>]><root/>`,
		`<!DOCTYPE root [<!ATTLIST root value CDATA '&amp;&#x41;&#65;'>]><root/>`,
		`<!DOCTYPE root [<!ENTITY uri 'urn:p'>]><root xmlns:p="&uri;"><p:item/></root>`,
		`<!DOCTYPE root [<!ENTITY unused '&missing;'><!ENTITY external SYSTEM 'root.ent'><!ENTITY cycle '&cycle;'>]><root/>`,
		`<p:root xmlns:p="urn:root"/>`,
		`<root xmlns:p="urn:root" p:id="one"/>`,
		`<root xmlns="urn:root"><child/></root>`,
		`<root xmlns="urn:root"><child xmlns=""/></root>`,
		`<a></a>`,
		`<root xml:lang="en"/>`,
		`<root id = "one"/>`,
		`<root xmlns:p="https://example.test/日本語"><p:item/></root>`,
		`<root xmlns:p="urn:example:%C3%A9"><p:item/></root>`,
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
		{name: "numeric whitespace before root", representation: "xml", content: "&#x20;<root/>"},
		{name: "numeric whitespace after root", representation: "xml", content: "<root/>&#x20;"},
		{name: "general entity whitespace before root", representation: "xml", content: `<!DOCTYPE root [<!ENTITY e " ">]>&e;<root/>`},
		{name: "general entity whitespace after root", representation: "xml", content: `<!DOCTYPE root [<!ENTITY e " ">]><root/>&e;`},
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
		{name: "doctype root mismatch", representation: "xml", content: "<!DOCTYPE other><root/>"},
		{name: "malformed element declaration", representation: "xml", content: "<!DOCTYPE root [<!ELEMENT root>]><root/>"},
		{name: "malformed attlist declaration", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root id>]><root/>"},
		{name: "attlist without default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root id CDATA>]><root/>"},
		{name: "truncated entity declaration", representation: "xml", content: "<!DOCTYPE root [<!ENTITY name \"value>]><root/>"},
		{name: "malformed entity declaration", representation: "xml", content: "<!DOCTYPE root [<!ENTITY name>]><root/>"},
		{name: "uppercase hexadecimal character reference", representation: "xml", content: "<!DOCTYPE root [<!ENTITY name '&#X41;'>]><root/>"},
		{name: "uppercase hexadecimal character reference in content", representation: "xml", content: "<root>&#X41;</root>"},
		{name: "undeclared entity use", representation: "xml", content: "<root>&missing;</root>"},
		{name: "external entity use", representation: "xml", content: "<!DOCTYPE root [<!ENTITY external SYSTEM 'root.ent'>]><root>&external;</root>"},
		{name: "internal entity with external dependency", representation: "xml", content: "<!DOCTYPE root [<!ENTITY external SYSTEM 'root.ent'><!ENTITY internal '&external;'>]><root>&internal;</root>"},
		{name: "cyclic entity use", representation: "xml", content: "<!DOCTYPE root [<!ENTITY first '&second;'><!ENTITY second '&first;'>]><root>&first;</root>"},
		{name: "forward entity default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&value;'><!ENTITY value 'forward'>]><root/>"},
		{name: "choice requires a second particle", representation: "xml", content: "<!DOCTYPE root [<!ELEMENT root (child|)>]><root/>"},
		{name: "undeclared parameter entity", representation: "xml", content: "<!DOCTYPE root [%parameter;]><root/>"},
		{name: "expanded markup duplicate attribute", representation: "xml", content: `<!DOCTYPE root [<!ENTITY markup '<child id="one" id="two"/>'>]><root>&markup;</root>`},
		{name: "expanded markup unbound prefix", representation: "xml", content: `<!DOCTYPE root [<!ENTITY markup '<p:child/>'>]><root>&markup;</root>`},
		{name: "expanded markup nested doctype", representation: "xml", content: `<!DOCTYPE root [<!ENTITY markup '<!DOCTYPE nested><child/>'>]><root>&markup;</root>`},
		{name: "expanded markup nested directive", representation: "xml", content: `<!DOCTYPE root [<!ENTITY markup '<!ENTITY nested "value">'>]><root>&markup;</root>`},
		{name: "expanded markup XML processing instruction", representation: "xml", content: `<!DOCTYPE root [<!ENTITY markup '<?xml version="1.0"?>'>]><root>&markup;</root>`},
		{name: "unescaped entity replacement end", representation: "xml", content: `<!DOCTYPE root [<!ENTITY value ']]>'>]><root>&value;</root>`},
		{name: "literal less-than entity in content", representation: "xml", content: "<!DOCTYPE root [<!ENTITY less '<'>]><root>&less;</root>"},
		{name: "character-reference less-than entity in content", representation: "xml", content: "<!DOCTYPE root [<!ENTITY less '&#60;'>]><root>&less;</root>"},
		{name: "literal less-than entity in actual attribute", representation: "xml", content: `<!DOCTYPE root [<!ENTITY less '<'>]><root value='&less;'/>`},
		{name: "character-reference less-than in actual attribute", representation: "xml", content: `<!DOCTYPE root [<!ENTITY less '&#60;'>]><root value='&less;'/>`},
		{name: "markup-bearing entity in actual attribute", representation: "xml", content: `<!DOCTYPE root [<!ENTITY markup '<child/>'>]><root value='&markup;'></root>`},
		{name: "character-reference less-than in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&less;'><!ENTITY less '&#60;'>]><root/>"},
		{name: "character-reference less-than in fixed attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA #FIXED '&less;'><!ENTITY less '&#60;'>]><root/>"},
		{name: "literal less-than in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&less;'><!ENTITY less '<'>]><root/>"},
		{name: "less-than in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '<'>]><root/>"},
		{name: "undeclared entity in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&missing;'>]><root/>"},
		{name: "external entity in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&external;'><!ENTITY external SYSTEM 'root.ent'>]><root/>"},
		{name: "markup-bearing entity in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&markup;'><!ENTITY markup '<child/>'>]><root/>"},
		{name: "cyclic entity in attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA '&first;'><!ENTITY first '&second;'><!ENTITY second '&first;'>]><root/>"},
		{name: "undeclared entity in fixed attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA #FIXED '&missing;'>]><root/>"},
		{name: "external entity in fixed attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA #FIXED '&external;'><!ENTITY external SYSTEM 'root.ent'>]><root/>"},
		{name: "markup-bearing entity in fixed attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA #FIXED '&markup;'><!ENTITY markup '<child/>'>]><root/>"},
		{name: "cyclic entity in fixed attribute default", representation: "xml", content: "<!DOCTYPE root [<!ATTLIST root value CDATA #FIXED '&first;'><!ENTITY first '&second;'><!ENTITY second '&first;'>]><root/>"},
		{name: "truncated notation declaration", representation: "xml", content: "<!DOCTYPE root [<!NOTATION image SYSTEM>]><root/>"},
		{name: "malformed notation declaration", representation: "xml", content: "<!DOCTYPE root [<!NOTATION image>]><root/>"},
		{name: "duplicate attribute", representation: "xml", content: `<root id="one" id="two"/>`},
		{name: "attributes without separator", representation: "xml", content: `<root id="one"name="two"/>`},
		{name: "invalid character reference", representation: "xml", content: `<root>&#xD800;</root>`},
		{name: "invalid attribute character reference", representation: "xml", content: `<root value="&#xD800;"/>`},
		{name: "duplicate expanded attribute", representation: "xml", content: `<root xmlns:a="urn:a" a:id="one" a:id="two"/>`},
		{name: "unbound element prefix", representation: "xml", content: "<p:root/>"},
		{name: "unbound attribute prefix", representation: "xml", content: `<root p:id="one"/>`},
		{name: "reserved element prefix", representation: "xml", content: "<xmlfoo:root/>"},
		{name: "reserved xmlns element prefix", representation: "xml", content: "<xmlns:root/>"},
		{name: "invalid element QName", representation: "xml", content: "<a::root/>"},
		{name: "reserved xml binding", representation: "xml", content: `<root xmlns:xml="urn:wrong"/>`},
		{name: "reserved xml prefix", representation: "xml", content: `<root xmlns:xmlfoo="urn:wrong"/>`},
		{name: "reserved xmlns binding", representation: "xml", content: `<root xmlns:xmlns="urn:wrong"/>`},
		{name: "reserved namespace URI", representation: "xml", content: `<root xmlns:p="http://www.w3.org/2000/xmlns/"/>`},
		{name: "reserved xml namespace URI", representation: "xml", content: `<root xmlns:p="http://www.w3.org/XML/1998/namespace"/>`},
		{name: "invalid namespace URI", representation: "xml", content: `<root xmlns:p="urn:with space"/>`},
		{name: "invalid namespace URI percent escape", representation: "xml", content: `<root xmlns:p="urn:bad%zz"/>`},
		{name: "invalid namespace URI control", representation: "xml", content: "<root xmlns:p=\"urn:bad\x7furi\"/>"},
		{name: "prefixed namespace undeclaration", representation: "xml", content: `<root xmlns:p="urn:p"><child xmlns:p=""><p:item/></child></root>`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			assertRejectedBootstrapXML(t, test)
		})
	}
}

func TestGenerateRejectsBootstrapXMLParameterEntitiesWithStableCause(t *testing.T) {
	contents := []string{
		`<!DOCTYPE root [<!ENTITY %parameter;>]><root/>`,
		`<!DOCTYPE root [<!ENTITY % parameter 'value'>%parameter;]><root/>`,
		`<!DOCTYPE root [<!ENTITY % parameter SYSTEM 'parameter.ent'>%parameter;]><root/>`,
		`<!DOCTYPE root [<!ENTITY value '%parameter;'>]><root/>`,
		`<!DOCTYPE root [<!ENTITY value 'text' %parameter;>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (%parameter;)>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (child %parameter;)>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (child|%parameter;)>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (#PCDATA|%parameter;)*>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root EMPTY %parameter;>]><root/>`,
		`<!DOCTYPE root [<!ATTLIST root %parameter;>]><root/>`,
		`<!DOCTYPE root [<!ATTLIST root id %parameter; #IMPLIED>]><root/>`,
		`<!DOCTYPE root [<!ATTLIST root id (one|%parameter;) #IMPLIED>]><root/>`,
		`<!DOCTYPE root [<!ENTITY value SYSTEM 'value.ent' %parameter;>]><root/>`,
		`<!DOCTYPE root [<!NOTATION image SYSTEM 'image' %parameter;>]><root/>`,
		`<!DOCTYPE root [%parameter;]><root/>`,
	}
	for _, content := range contents {
		t.Run(content, func(t *testing.T) {
			assertBootstrapXMLFailureCause(t, content, errBootstrapXMLParameterEntityUnsupported)
		})
	}
}

func TestGenerateAcceptsUnusedParameterEntityDeclarations(t *testing.T) {
	contents := []string{
		`<!DOCTYPE root [<!ENTITY % parameter 'value'>]><root/>`,
		`<!DOCTYPE root [<!ENTITY % parameter SYSTEM 'parameter.ent'>]><root/>`,
		`<!DOCTYPE schema [<!ENTITY % schemaAttrs 'xmlns:hfp CDATA #IMPLIED'><!ELEMENT schema EMPTY>]><schema/>`,
	}
	for _, content := range contents {
		t.Run(strconv.Itoa(len(content)), func(t *testing.T) {
			assertAcceptedBootstrapXML(t, content)
		})
	}
}

func TestGenerateRejectsExternalSubsetEntityUseWithStableCause(t *testing.T) {
	assertBootstrapXMLFailureCause(t, `<!DOCTYPE root SYSTEM "root.dtd"><root>&external;</root>`,
		errBootstrapXMLExternalEntityUnsupported)
}

func TestBootstrapXMLAttributeDefaultEntityCutoff(t *testing.T) {
	assertAcceptedBootstrapXML(t,
		`<!DOCTYPE root [<!ENTITY value "ready"><!ENTITY alias "&value;"><!ATTLIST root data CDATA "&alias;">]><root/>`)
	assertBootstrapXMLFailureCause(t,
		`<!DOCTYPE root [<!ENTITY alias "&value;"><!ATTLIST root data CDATA "&alias;"><!ENTITY value "late">]><root/>`,
		errBootstrapXMLUnknownEntity)
	assertBootstrapXMLFailureCause(t,
		`<!DOCTYPE root SYSTEM "root.dtd" [<!ATTLIST root data CDATA "&missing;">]><root/>`,
		errBootstrapXMLExternalEntityUnsupported)
}

func TestBootstrapXMLPredefinedEntityDeclarations(t *testing.T) {
	valid := []string{
		`<!DOCTYPE root [<!ENTITY lt "&#38;#60;"><!ENTITY amp "&#38;#38;"><!ENTITY gt ">"><!ENTITY apos "'"><!ENTITY quot '"'>]><root/>`,
		`<!DOCTYPE root [<!ENTITY lt "&#x26;#x3C;"><!ENTITY amp "&#x26;#x26;"><!ENTITY gt "&#x3E;"><!ENTITY apos "&#x27;"><!ENTITY quot "&#x22;">]><root/>`,
	}
	for _, content := range valid {
		t.Run("valid/"+strconv.Itoa(len(content)), func(t *testing.T) {
			assertAcceptedBootstrapXML(t, content)
		})
	}
	invalid := []string{
		`<!DOCTYPE root [<!ENTITY lt "<">]><root/>`,
		`<!DOCTYPE root [<!ENTITY lt "&#60;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY lt "&lt;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY lt SYSTEM "lt.ent">]><root/>`,
		`<!DOCTYPE root [<!ENTITY amp "&#38;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY amp "&#38;#60;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY gt "&gt;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY gt "&#38;#62;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY apos "&#34;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY quot "&#39;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY lt "&#38;#60;"><!ENTITY lt "<">]><root/>`,
	}
	for _, content := range invalid {
		t.Run("invalid/"+strconv.Itoa(len(content)), func(t *testing.T) {
			assertRejectedBootstrapXML(t, bootstrapXMLInvalidCase{
				name:           "predefined entity declaration",
				representation: "xml",
				content:        content,
			})
		})
	}
}

func TestGenerateRemapsEntityFragmentFailureToOuterLocation(t *testing.T) {
	content := "<!DOCTYPE root [<!ENTITY fragment '<child>'>]>" + "\n" + "<root>&fragment;</root>"
	raw := []byte(content)
	entry := testEntry("xml", testDigest(raw))
	entry.Kind = KindBootstrapArtifact
	_, err := Generate(context.Background(), responseClient(raw), entry)
	if err == nil {
		t.Fatal("Generate() error = nil")
	}
	var syntaxErr *xml.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("Generate() error = %v, want XML syntax cause", err)
	}
	if syntaxErr.Line != 2 {
		t.Fatalf("Generate() syntax line = %d, want 2", syntaxErr.Line)
	}
	var corpusErr *Error
	if !errors.As(err, &corpusErr) || corpusErr.ID != entry.ID || corpusErr.URL != entry.URL {
		t.Fatalf("Generate() error = %v, want outer entry location", err)
	}
}

func TestGenerateBootstrapXMLDepthLimitsPreserveResourceCause(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "general entity chain",
			content: bootstrapXMLDeepEntityDocument(bootstrapXMLMaxEntityDepth),
		},
		{
			name:    "nested content groups",
			content: bootstrapXMLDeepGroupDocument(bootstrapXMLMaxContentGroupDepth + 1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertBootstrapXMLFailureCause(t, test.content, errBootstrapXMLResourceLimit)
		})
	}
}

func TestGenerateAcceptsBootstrapXMLAtDepthLimits(t *testing.T) {
	contents := []string{
		bootstrapXMLDeepEntityDocument(bootstrapXMLMaxEntityDepth - 1),
		bootstrapXMLDeepGroupDocument(bootstrapXMLMaxContentGroupDepth - 1),
	}
	for _, content := range contents {
		t.Run(fmt.Sprintf("length=%d", len(content)), func(t *testing.T) {
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

func TestGenerateRejectsRepeatedEntityExpansionOverSharedBudget(t *testing.T) {
	content := "<!DOCTYPE root [<!ENTITY e 'x'>]><root>" + strings.Repeat("&e;", bootstrapXMLMaxEntityValueLength+1) + "</root>"
	assertBootstrapXMLFailureCause(t, content, errBootstrapXMLResourceLimit)
}

func assertBootstrapXMLFailureCause(t *testing.T, content string, cause error) {
	t.Helper()
	raw := []byte(content)
	entry := testEntry("xml", testDigest(raw))
	entry.Kind = KindBootstrapArtifact
	document, err := Generate(context.Background(), responseClient(raw), entry)
	if err == nil {
		t.Fatal("Generate() error = nil")
	}
	if document.Data != nil || document.Index != nil || document.Entry.ID != "" {
		t.Fatalf("Generate() document = %#v, want zero document", document)
	}
	assertErrorCode(t, err, bootstrapXMLDocumentCode)
	if !errors.Is(err, cause) {
		t.Fatalf("Generate() error = %v, want cause %v", err, cause)
	}
}

func bootstrapXMLDeepEntityDocument(depth int) string {
	var declarations strings.Builder
	for index := depth; index >= 0; index-- {
		if index == 0 {
			declarations.WriteString("<!ENTITY e0 'ok'>")
			continue
		}
		declarations.WriteString("<!ENTITY e" + strconv.Itoa(index) + " '&e" + strconv.Itoa(index-1) + ";'>")
	}
	return fmt.Sprintf("<!DOCTYPE root [%s]><root>&e%d;</root>", declarations.String(), depth)
}

func bootstrapXMLDeepGroupDocument(depth int) string {
	content := "child"
	for index := 0; index < depth; index++ {
		content = "(" + content + ")"
	}
	return "<!DOCTYPE root [<!ELEMENT root " + content + ">]><root/>"
}

func TestGenerateRejectsOverLimitEntityInAttributeDefaults(t *testing.T) {
	largeValue := strings.Repeat("x", bootstrapXMLMaxEntityValueLength+1)
	for _, fixed := range []bool{false, true} {
		defaultDeclaration := "CDATA "
		if fixed {
			defaultDeclaration += "#FIXED "
		}
		content := "<!DOCTYPE root [<!ATTLIST root value " + defaultDeclaration + "'&large;'><!ENTITY large '" + largeValue + "'>]><root/>"
		t.Run(defaultDeclaration, func(t *testing.T) {
			assertRejectedBootstrapXML(t, bootstrapXMLInvalidCase{
				name:           "over-limit entity in attribute default",
				representation: "xml",
				content:        content,
			})
		})
	}
}

func TestBootstrapXMLEntityUseContexts(t *testing.T) {
	valid := []string{
		`<!DOCTYPE root [<!ENTITY lt "&#38;#60;"><!ENTITY numeric "&#38;#60;"><!ENTITY escaped "&amp;#60;"><!ENTITY foo "declared"><!ENTITY unknown "&#38;foo;"><!ENTITY missing "&not-declared;"><!ENTITY external SYSTEM "external.ent"><!ENTITY cycle "&cycle;"><!ENTITY fragment "<p:item/>"><!ENTITY uri "urn:p"><!ENTITY comment "<!-- &not-declared; -->"><!ENTITY pi "<?p &not-declared;?>"><!ENTITY cdata "<![CDATA[&not-declared;]]>">]><root xmlns:p="&uri;" a="&lt;" b="&#38;#60;" c="&amp;#60;" d="&unknown;">&lt;&#60;&lt;&numeric;&escaped;&unknown;&fragment;&comment;&pi;&cdata;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<p:item/>">]><root xmlns:p="urn:p"><holder>&fragment;</holder></root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<p:item xmlns:p='urn:inner'/>">]><root xmlns:p="urn:outer">&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY x "&lt;"><!ENTITY numeric "&#38;#60;"><!ATTLIST root a CDATA "&x;">]><root a="&x;" b="&numeric;">&x;&numeric;</root>`,
		`<root><!-- &missing; --><child><?pi &missing;?></child><![CDATA[&missing;]]></root>`,
		`<root>]]&gt;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "]]">]><root>&fragment;></root>`,
		`<!DOCTYPE root [<!ENTITY left "]"><!ENTITY right "]">]><root>&left;&right;></root>`,
		`<!DOCTYPE root [<!ENTITY end "&gt;">]><root>]]&end;</root>`,
		`<!DOCTYPE root [<!ENTITY end ">">]><root>]]&end;</root>`,
		`<!DOCTYPE root [<!ENTITY end "&#62;">]><root>]]&end;</root>`,
		`<!DOCTYPE root [<!ENTITY end "&#38;#62;">]><root>]]&end;</root>`,
	}
	for _, content := range valid {
		t.Run("valid/"+strconv.Itoa(len(content)), func(t *testing.T) {
			assertAcceptedBootstrapXML(t, content)
		})
	}

	invalid := []string{
		`<!DOCTYPE root [<!ENTITY fragment "<root/>">]>&fragment;`,
		`<!DOCTYPE root [<!ENTITY nested "<root/>"><!ENTITY fragment "&nested;">]>&fragment;`,
		`<!DOCTYPE root [<!ENTITY empty ""><!ENTITY comment "<!-- comment -->"><!ENTITY pi "<?p &missing;?>">]>&comment;&empty;<root/>&pi;`,
		`<!DOCTYPE root [<!ENTITY literal "<"><!ENTITY numeric "&#60;">]><root a="&literal;"/>`,
		`<!DOCTYPE root [<!ENTITY literal "<"><!ENTITY numeric "&#60;">]><root a="&numeric;"/>`,
		`<!DOCTYPE root [<!ENTITY literal "<"><!ATTLIST root a CDATA "&literal;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY numeric "&#60;"><!ATTLIST root a CDATA "&numeric;">]><root/>`,
		`<!DOCTYPE root [<!ENTITY missing "&not-declared;">]><root>&missing;</root>`,
		`<!DOCTYPE root [<!ENTITY e "&#38;foo;">]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY e "&#38;x">]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY e "&#38;">]><root>&e;</root>`,
		`<!DOCTYPE root [<!ENTITY external SYSTEM "external.ent">]><root>&external;</root>`,
		`<!DOCTYPE root [<!ENTITY first "&second;"><!ENTITY second "&first;">]><root>&first;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<p:item/>">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<child>">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<other/>">]>&fragment;`,
		`<!DOCTYPE root [<!ENTITY fragment "<p:item/>">]><root xmlns:q="urn:p">&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY uri "urn:bad space">]><root xmlns:p="&uri;"/>`,
		`<!DOCTYPE root [<!ENTITY uri "&missing;">]><root xmlns:p="&uri;"/>`,
		`<!DOCTYPE root [<!ENTITY fragment '<p:item xmlns:p="urn:p" p:id="one" q:id="two" xmlns:q="urn:p"/>'>]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<!DOCTYPE nested><child/>">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<!ENTITY nested 'value'>">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<?xml version='1.0'?>">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<!-- --&gt;">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<?pi ?&gt;">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<![CDATA[]]&gt;">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "]]>">]><root>&fragment;</root>`,
		`<!DOCTYPE root [<!ENTITY fragment "<![CDATA[]]><root/>">]>&fragment;`,
	}
	for index, content := range invalid {
		content := content
		t.Run("invalid/"+strconv.Itoa(index)+"/"+strconv.Itoa(len(content)), func(t *testing.T) {
			assertRejectedBootstrapXML(t, bootstrapXMLInvalidCase{name: "entity context", representation: "xml", content: content})
		})
	}
}

func assertAcceptedBootstrapXML(t *testing.T, content string) {
	t.Helper()
	raw := []byte(content)
	entry := testEntry("xml", testDigest(raw))
	entry.Kind = KindBootstrapArtifact
	document, err := Generate(context.Background(), responseClient(raw), entry)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !bytes.Equal(document.Data, raw) {
		t.Fatalf("Generate() data = %q, want %q", document.Data, raw)
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

func xsd10DatatypesTestEntry(digest string) Entry {
	entry := testEntry(manifestXSD10DatatypesRepresentation, digest)
	entry.ID = xsd10DatatypesSchemaID
	entry.Kind = KindBootstrapArtifact
	entry.XSDVersions = []string{"1.0"}
	return entry
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
